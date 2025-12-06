package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Adapter is the marker type for all adapters.
type Adapter any

// ZeroFactory should construct a zero-cost, zero-valued adapter.
type ZeroFactory func() Adapter

type Registry struct {
	mu        sync.RWMutex
	factories map[string]ZeroFactory
	adapters  map[string]Adapter
	searchMap *SearchMap
}

var defaultRegistry = &Registry{
	factories: make(map[string]ZeroFactory),
	adapters:  make(map[string]Adapter),
}

// DefaultRegistry returns the package-global registry used by the helper funcs.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// SetSearchMap sets the SearchMap used by this registry.
// In typical CLI usage it's set once at startup; we don't worry about races here.
func (r *Registry) SetSearchMap(sm *SearchMap) {
	r.searchMap = sm
}

// SetSearchPath constructs a SearchMap rooted at the given path and installs it
// into this registry. It returns the created SearchMap.
func (r *Registry) SetSearchPath(root string) (*SearchMap, error) {
	sm, err := NewSearchMap(root)
	if err != nil {
		return nil, err
	}
	r.searchMap = sm
	return sm, nil
}

// Convenience: configure the default registry's search path.
func SetDefaultSearchPath(root string) (*SearchMap, error) {
	return defaultRegistry.SetSearchPath(root)
}

// (Optional) Lower-level convenience if you already built a SearchMap yourself.
func SetDefaultSearchMap(sm *SearchMap) {
	defaultRegistry.SetSearchMap(sm)
}

// Register adds an Adapter constructor to the registry.
func (r *Registry) Register(adapterID string, f ZeroFactory) {
	r.mu.Lock()
	r.factories[strings.ToLower(adapterID)] = f
	r.mu.Unlock()
}

// IsRegistered reports whether an adapterID has a registered factory.
func (r *Registry) IsRegistered(adapterID string) bool {
	r.mu.RLock()
	_, ok := r.factories[strings.ToLower(adapterID)]
	r.mu.RUnlock()
	return ok
}

func (r *Registry) getFactory(adapterID string) (ZeroFactory, error) {
	r.mu.RLock()
	zeroFac, ok := r.factories[strings.ToLower(adapterID)]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown adapter %q", adapterID)
	}
	return zeroFac, nil
}

// applyDeps wires dependencies into an adapter using both map-style (Depender) and
// struct-field injection.
func applyDeps(adapter Adapter, meta *MetaHeader) error {
	if meta == nil || meta.Dependencies == nil {
		return nil
	}

	if depender, ok := adapter.(Depender); ok {
		if err := resolveMapDeps(depender, meta.Dependencies); err != nil {
			return err
		}
	}
	if err := resolveStructDeps(adapter, meta.Dependencies); err != nil {
		return err
	}
	return nil
}

// applyContext calls SetContext for each meta that has Context set.
// Order matters: later metas override earlier ones.
func applyContext(adapter Adapter, metas ...*MetaHeader) {
	contextual, ok := adapter.(Contextual)
	if !ok {
		return
	}
	for _, m := range metas {
		if m == nil {
			continue
		}
		Log().Debugf("Setting context for adapter %s: %s\n", m.Name, m.Context)
		if m.Context != "" {
			contextual.SetContext(m.Context)
		}
	}
}

func debugAdapterInfo(zero Adapter, adapterID string, args ...string) {
	implements := []string{}
	if _, ok := zero.(Configurable); ok {
		implements = append(implements, "Configurable")
	}
	if _, ok := zero.(ItemConfigurable); ok {
		implements = append(implements, "ItemConfigurable")
	}
	if _, ok := zero.(Hydrater); ok {
		implements = append(implements, "Hydrater")
	}
	if _, ok := zero.(Contextual); ok {
		implements = append(implements, "Contextual")
	}
	if _, ok := zero.(Depender); ok {
		implements = append(implements, "Depender")
	}
	Log().Debugf("Request adapter %s (%s) %v\n", adapterID, strings.Join(implements, ","), args)
}

// NewAdapter constructs or reuses an adapter instance in this registry.
func (r *Registry) NewAdapter(adapterID string, args ...string) (Adapter, error) {
	if r.searchMap == nil {
		return nil, fmt.Errorf("core: no SearchMap configured; call NewSearchMap first")
	}

	zeroFac, err := r.getFactory(adapterID)
	if err != nil {
		return nil, err
	}

	zero := zeroFac()

	debugAdapterInfo(zero, adapterID, args...)

	var meta *MetaHeader
	var itemMeta *MetaHeader

	// Adapter-level config (optional).
	meta, err = r.searchMap.Load(adapterID, true)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed reading config for adapter %s: %v", adapterID, err)
	}

	// Item-level config (optional, if adapter supports it and args provided).
	if _, isItemConfigurable := zero.(ItemConfigurable); isItemConfigurable && len(args) > 0 {
		configPath := args[0]
		itemMeta, err = r.searchMap.Load(configPath, true)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed reading item config: %s for adapter %s: %v", configPath, adapterID, err)
		}
	}

	// Compute registry cache key.
	regKey := strings.ToLower(adapterID)
	if itemMeta != nil && itemMeta.Name != "" {
		regKey = regKey + "__" + itemMeta.Name
	}

	// Reuse existing adapter if present.
	r.mu.RLock()
	existing, ok := r.adapters[regKey]
	r.mu.RUnlock()
	if ok {
		Log().Debugf("Reusing adapter: %s %v\n", adapterID, args)
		return existing, nil
	}

	// Otherwise create a new instance.
	Log().Debugf("Creating adapter: %s %v\n", adapterID, args)
	adapter := zero

	r.mu.Lock()
	r.adapters[regKey] = adapter
	r.mu.Unlock()

	// Adapter-level config.
	if meta != nil && len(meta.RawSpec) > 0 {
		if configurable, ok := adapter.(Configurable); ok {
			Log().Debugf("Setting config for adapter %s", adapterID)
			if err := json.Unmarshal(meta.RawSpec, configurable.ConfigPtr()); err != nil {
				return nil, fmt.Errorf("decode %s spec: %w", adapterID, err)
			}
		}
	}

	// Item-level config overlay.
	if itemMeta != nil && len(itemMeta.RawSpec) > 0 {
		if itemConfigurable, ok := adapter.(ItemConfigurable); ok {
			Log().Debugf("Setting item config for adapter %s", adapterID)
			if err := json.Unmarshal(itemMeta.RawSpec, itemConfigurable.ItemConfigPtr(itemMeta.Name)); err != nil {
				return nil, fmt.Errorf("decode %s spec: %w", itemMeta.Name, err)
			}
		}
	}

	// Contexts (adapter-level then item-level).
	applyContext(adapter, meta, itemMeta)

	// Dependencies.
	if err := applyDeps(adapter, meta); err != nil {
		return nil, fmt.Errorf("dependency resolution for %s: %w", adapterID, err)
	}
	if err := applyDeps(adapter, itemMeta); err != nil {
		return nil, fmt.Errorf("dependency resolution for %s: %w", adapterID, err)
	}

	// Required dependency validation.
	if err := validateRequiredDeps(adapter); err != nil {
		return nil, fmt.Errorf("validating adapter %s: %w", adapterID, err)
	}

	// Hydration hook.
	if hydrater, ok := adapter.(Hydrater); ok {
		Log().Debugf("Hydrating adapter: %s\n", adapterID)
		if err := hydrater.Hydrate(context.Background()); err != nil {
			return nil, fmt.Errorf("hydrating adapter %s: %v", adapterID, err)
		}
	}

	return adapter, nil
}

// loadAllMetas is a small helper to retrieve all MetaHeaders for an adapter ID.
func (r *Registry) loadAllMetas(adapterID string) ([]*MetaHeader, error) {
	if r.searchMap == nil {
		return nil, fmt.Errorf("core: no SearchMap configured; call NewSearchMap first")
	}
	return r.searchMap.LoadAll(adapterID)
}

// --- Generic helpers (functions, not methods) ---

// NewAdapterAsFrom constructs an adapter from the given registry and asserts it implements T.
func NewAdapterAsFrom[T any](r *Registry, adapterID string, args ...string) (T, error) {
	var zeroT T

	a, err := r.NewAdapter(adapterID, args...)
	if err != nil {
		return zeroT, err
	}
	t, ok := a.(T)
	if ok {
		return t, nil
	}

	return zeroT, fmt.Errorf(
		"adapter %q does not implement requested type: expected %T, got %T",
		adapterID, zeroT, a,
	)
}

// LoadAllAdaptersFrom loads all configured items for adapterID from the given registry
// and returns them as []T, skipping items that fail type assertion or construction.
func LoadAllAdaptersFrom[T any](r *Registry, adapterID string) ([]T, error) {
	metas, err := r.loadAllMetas(adapterID)
	if err != nil {
		return nil, err
	}

	var out []T
	for _, meta := range metas {
		a, err := NewAdapterAsFrom[T](r, adapterID, meta.Name)
		if err != nil {
			Log().Errorf("Error: %v\n", err)
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// Adapters returns a shallow copy of the cached adapters of the default registry.
// (Mostly for debugging / introspection.)
func Adapters() map[string]Adapter {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()

	cp := make(map[string]Adapter, len(defaultRegistry.adapters))
	for k, v := range defaultRegistry.adapters {
		cp[k] = v
	}
	return cp
}

// --- Package-level helpers using the default registry ---

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
	defaultRegistry.Register(adapterID, f)
}

func IsRegistered(adapterID string) bool {
	return defaultRegistry.IsRegistered(adapterID)
}

func NewAdapter(adapterID string, args ...string) (Adapter, error) {
	return defaultRegistry.NewAdapter(adapterID, args...)
}

// NewAdapterAs constructs an adapter from the default registry and asserts it implements T.
func NewAdapterAs[T any](adapterID string, args ...string) (T, error) {
	return NewAdapterAsFrom[T](defaultRegistry, adapterID, args...)
}

// LoadAllAdapters loads all configured items for adapterID from the default registry.
func LoadAllAdapters[T any](adapterID string) ([]T, error) {
	return LoadAllAdaptersFrom[T](defaultRegistry, adapterID)
}
