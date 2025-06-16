package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

type ZeroFactory func() Adapter

var (
	searchPath = "./.core/"
	mu         sync.RWMutex
	factories  = map[string]ZeroFactory{} // adapterID -> Factory
	adapters   = map[string]Adapter{}     // configName -> Adapter
	contextMap = map[string]string{}
	searchMap  *SearchMap
)

func Adapters() map[string]Adapter {
	return adapters
}

func SetSearchPath(path string) {
	searchPath = path
}

func init() {
	contextMapJSON := os.Getenv("CORE_CONTEXT_MAP")
	if contextMapJSON != "" {
		_ = json.Unmarshal([]byte(contextMapJSON), &contextMap)
	}
	envPath := os.Getenv("CORE_SEARCH_PATH")
	if envPath != "" {
		searchPath = envPath
	}
	fmt.Printf("Search path: %s\n", searchPath)
	var err error
	searchMap, err = NewSearchMap(searchPath)
	if err != nil {
		log.Fatalf("failed to index configs: %v", err)
	}
}

// Register adds an Adapter constructor to the global registry.
//
// fn MUST return a Adapter that is fully zero-initialised.
// There should be no heavy lifting
//
//	func init() {
//		core.Register("adapter-id", func() Adapter {
//			return &GCloud{} // zero cost constructor
//		})
//	}
//
// Later the framework will do:
//
//	a := registry["adapter-id"]()   // clone via factory
//
//	if c, ok := a.(Configurable); ok {
//		cfg := c.ConfigPtr()     // returns pointer to struct
//		loadJSON(cfg)            // unmarshal adapter-level config
//	}
//
//	if ic, ok := a.(ItemConfigurable); ok {
//		itemCfg := ic.ItemConfigPtr() // pointer to per-item struct
//		loadItemJSON(itemID, itemCfg) // unmarshals one item
//	}
//
// and run the adapter
func Register(adapterID string, f ZeroFactory) {
	mu.Lock()
	factories[strings.ToLower(adapterID)] = f
	mu.Unlock()
}

func IsRegistered(adapterID string) bool {
	_, ok := factories[strings.ToLower(adapterID)]
	return ok
}

func NewAdapter(adapterID string, args ...string) (Adapter, error) {

	mu.RLock()
	zeroFac, ok := factories[strings.ToLower(adapterID)]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown adapter %q", adapterID)
	}

	zero := zeroFac()

	var meta *MetaHeader
	var itemMeta *MetaHeader

	// Attempt to find generic adapter configuration
	// LoadMeta will handle name resolving
	meta, err := searchMap.Load(adapterID, true)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed reading config for adapter %s: %v", adapterID, err)
	}

	if _, isItemConfigurable := zero.(ItemConfigurable); isItemConfigurable && len(args) > 0 {
		configPath := args[0]
		// args = args[1:] // strip of adapter ID arg
		// Attempt to find configuration
		// LoadMeta will handle name resolving
		itemMeta, err = searchMap.Load(configPath, true)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed reading item config: %s for adapter %s: %v", configPath, adapterID, err)
		}
	}

	var regKey string = strings.ToLower(adapterID)
	if itemMeta != nil && itemMeta.Name != "" {
		regKey = regKey + "__" + itemMeta.Name
	}

	// Check for exsiting that should be reused
	mu.RLock()
	existing, ok := adapters[regKey]
	mu.RUnlock()
	if ok {
		fmt.Printf("Reusing adapter: %s %v\n", adapterID, args)
		return existing, nil
	}

	// Otherwise use the zero instance
	fmt.Printf("Creating adapter: %s %v\n", adapterID, args)

	adapter := zero

	mu.Lock()
	adapters[regKey] = adapter
	mu.Unlock()

	// Check if configurable
	if meta != nil && len(meta.RawSpec) > 0 {
		if configurable, isConfigurable := adapter.(Configurable); isConfigurable {
			if err := json.Unmarshal(meta.RawSpec, configurable.ConfigPtr()); err != nil {
				return nil, fmt.Errorf("decode %s spec: %w", adapterID, err)
			}
		}
	}

	// Check if item configurable
	if itemMeta != nil && len(itemMeta.RawSpec) > 0 {
		if itemConfigurable, isItemConfigurable := adapter.(ItemConfigurable); isItemConfigurable {
			if err := json.Unmarshal(itemMeta.RawSpec, itemConfigurable.ItemConfigPtr(itemMeta.Name)); err != nil {
				return nil, fmt.Errorf("decode %s spec: %w", itemMeta.Name, err)
			}
		}
	}

	// Check if contextual
	if contextual, isContextual := adapter.(Contextual); isContextual {
		if meta != nil && meta.Context != "" {
			contextual.SetContext(meta.Context)
		}
		if itemMeta != nil && itemMeta.Context != "" {
			contextual.SetContext(itemMeta.Context)
		}
	}

	// Check if depender
	if depender, isDepender := adapter.(Depender); isDepender {
		if meta != nil && meta.Dependencies != nil {
			if err := resolveMapDeps(depender, meta.Dependencies); err != nil {
				return nil, fmt.Errorf("dependency resolution: %w", err)
			}
		}
		if itemMeta != nil && itemMeta.Dependencies != nil {
			if err := resolveMapDeps(depender, itemMeta.Dependencies); err != nil {
				return nil, fmt.Errorf("dependency resolution: %w", err)
			}
		}
	}

	// Add the struct-field injector (safe for non-struct parents).
	if meta != nil && meta.Dependencies != nil {
		if err := resolveStructDeps(adapter, meta.Dependencies); err != nil {
			return nil, fmt.Errorf("dependency resolution for %s: %w", adapterID, err)
		}
	}

	if itemMeta != nil && itemMeta.Dependencies != nil {
		if err := resolveStructDeps(adapter, itemMeta.Dependencies); err != nil {
			return nil, fmt.Errorf("dependency resolution for %s: %w", adapterID, err)
		}
	}

	if err := validateRequiredDeps(adapter); err != nil {
		return nil, fmt.Errorf("validating adapter %s: %w", adapterID, err)
	}

	if hydrater, isHydratable := adapter.(Hydrater); isHydratable {
		fmt.Printf("Hydrating adapter: %s\n", adapterID)
		err := hydrater.Hydrate(context.Background())
		if err != nil {
			return nil, fmt.Errorf("hydrating adapter %s: %v", adapterID, err)
		}
	}

	return adapter, nil
}

func NewAdapterAs[T any](adapterID string, args ...string) (T, error) {
	var zero T

	a, err := NewAdapter(adapterID, args...)
	if err != nil {
		return zero, err
	}
	t, ok := a.(T)
	if ok {
		return t, nil
	}

	return zero, fmt.Errorf(
		"adapter %q does not implement requested type: expected %T, got %T",
		adapterID, zero, a,
	)
}
