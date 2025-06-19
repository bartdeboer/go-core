package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	searchMap *SearchMap
)

type DepRef struct {
	Adapter string   `json:"adapter"`        // required
	Name    string   `json:"name,omitempty"` // fallback on config name
	Args    []string `json:"args,omitempty"` // extra CLI-style args
}

// The Context is managed by the system to ensure those paths are adjusted
// accordingly when the system runs in a container (with volume mounts).
// Adapters still need to use the value manually to use the context.

type MetaHeader struct {
	Name         string            `json:"name"` // fallback on filename
	APIVersion   string            `json:"api_version"`
	Adapter      string            `json:"adapter,omitempty"`
	Dependencies map[string]DepRef `json:"dependencies"`
	RawSpec      json.RawMessage   `json:"spec"`    // adapter-specific payload
	Context      string            `json:"context"` // project path (rel or abs)
}

type SearchMap struct {
	root  string
	Short map[string][]string // basename (no .json) -> []absolute paths
	Full  map[string]string   // relative/key (no .json) -> absolute path
}

func init() {
	contextMapJSON := os.Getenv("CORE_CONTEXT_MAP")
	if contextMapJSON != "" {
		_ = json.Unmarshal([]byte(contextMapJSON), &contextMap)
	}
}

// NewSearchMap walks root and builds both Short and Full indexes.
func NewSearchMap(root string) (*SearchMap, error) {

	searchMap = &SearchMap{
		root:  root,
		Short: make(map[string][]string),
		Full:  make(map[string]string),
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(d.Name()) != ".json" {
			return nil
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("resolve abs path %q: %w", path, err)
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relativize %q: %w", path, err)
		}
		relKey := strings.TrimSuffix(rel, ".json")
		searchMap.Full[relKey] = absPath

		shortKey := strings.TrimSuffix(d.Name(), ".json")
		searchMap.Short[shortKey] = append(searchMap.Short[shortKey], absPath)
		return nil
	})
	if err != nil {
		return searchMap, err
	}
	return searchMap, nil
}

// Resolve finds the one absolute path for name.
// name can be either the short key ("dev") or full key ("env/dev").
func (sm *SearchMap) Resolve(name string) (string, error) {

	// Try full-key first
	if p, ok := sm.Full[name]; ok {
		return p, nil
	}

	// Then short-key
	list, ok := sm.Short[name]
	if !ok || len(list) == 0 {
		return "", os.ErrNotExist
	}
	if len(list) > 1 {
		return "", fmt.Errorf(
			"ambiguous config %q matches:\n  - %s",
			name, strings.Join(list, "\n  - "),
		)
	}
	return list[0], nil
}

// Load locates, reads, unmarshals and post-processes a MetaHeader.
func (sm *SearchMap) Load(name string, verbose bool) (*MetaHeader, error) {
	p, err := sm.Resolve(name)
	if err != nil {
		return nil, err
	}

	if verbose {
		fmt.Printf("Reading %s config: %s\n", name, p)
	}

	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}

	var h MetaHeader
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("decode %s: %w", p, err)
	}

	if strings.TrimSpace(h.Name) == "" {
		h.Name = strings.TrimSuffix(filepath.Base(p), ".json")
	}

	// Override from env/contextMap if present
	if envCtx, ok := contextMap[h.Name]; ok {
		h.Context = filepath.Clean(envCtx)
	}

	// Make Context absolute if it’s relative
	if h.Context != "" && !filepath.IsAbs(h.Context) {
		dir := filepath.Dir(p)
		absCtx, err := filepath.Abs(filepath.Join(dir, h.Context))
		if err != nil {
			return nil, fmt.Errorf("resolve context %q: %w", h.Context, err)
		}
		h.Context = filepath.Clean(absCtx)
	}

	return &h, nil
}

// LoadAll walks through every indexed config, loads it, and
// returns those whose Adapter matches adapterID (or all if adapterID=="").
func (sm *SearchMap) LoadAll(adapterID string) ([]*MetaHeader, error) {
	// Collect keys in deterministic order
	keys := make([]string, 0, len(sm.Full))
	for k := range sm.Full {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result []*MetaHeader
	for _, key := range keys {
		meta, err := sm.Load(key, false)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("Could not find config for: %s\n", key)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("error loading meta %q: %w", key, err)
		}
		if adapterID != "" && !strings.EqualFold(meta.Adapter, adapterID) {
			continue
		}
		result = append(result, meta)
	}
	return result, nil
}

func LoadAll(adapterID string) ([]*MetaHeader, error) {
	return searchMap.LoadAll(adapterID)
}
