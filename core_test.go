package core_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	core "github.com/bartdeboer/go-core"
)

// ListerAdp implements core.Lister and is configurable via lister-adp.json.
type ListerAdp struct {
	Note        string `json:"note"`
	ContextPath string
}

func (l *ListerAdp) List(ctx context.Context) ([]string, error) {
	return []string{"one", "two"}, nil
}

// ConfigPtr makes ListerAdp implement core.Configurable so lister-adp.json is used.
func (l *ListerAdp) ConfigPtr() any {
	return l
}

// SetContext makes ListerAdp implement core.Contextual.
func (l *ListerAdp) SetContext(path string) {
	l.ContextPath = path
}

// Adp is the main adapter under test.
// It uses a single Spec struct for both adapter-level and item-level config.
// Item config should override adapter config fields.
type Adp struct {
	Spec struct {
		Foo   string `json:"foo"`
		Label string `json:"label"`
	}

	ContextPath string

	// Injected from dependencies in adp.json:
	// "dependencies": { "ListerProvider": { "adapter": "lister-adp" } }
	ListerProvider core.Lister `core:"required"`
}

// ConfigPtr makes Adp implement core.Configurable.
func (a *Adp) ConfigPtr() any {
	return &a.Spec
}

// ItemConfigPtr makes Adp implement core.ItemConfigurable.
// We deliberately return the same Spec pointer so item config overlays adapter config.
func (a *Adp) ItemConfigPtr(name string) any {
	return &a.Spec
}

// SetContext makes Adp implement core.Contextual.
func (a *Adp) SetContext(path string) {
	a.ContextPath = path
}

func TestAdapter_ConfigOverride_DependencyInjection_AndContext(t *testing.T) {
	// Use configs from ./testdata.
	if _, err := core.SetDefaultSearchPath("testdata"); err != nil {
		t.Fatalf("SearchMap: %v", err)
	}

	// Register adapters.
	core.Register("lister-adp", func() core.Adapter { return &ListerAdp{} })
	core.Register("adp", func() core.Adapter { return &Adp{} })

	// Create an instance of "adp" using the item config "items/inst1".
	// Uses:
	//   - testdata/adp.json         (adapter-level config + context)
	//   - testdata/items/inst1.json (item-level config + context)
	//   - testdata/lister-adp.json  (dependency config + context)
	adp, err := core.NewAdapterAs[*Adp]("adp", "items/inst1")
	if err != nil {
		t.Fatalf("NewAdapterAs(adp): %v", err)
	}

	// --- Config overlay behavior ---
	// From adp.json:
	//   "foo": "global-foo"
	// From items/inst1.json:
	//   "foo": "item-foo"
	//   "label": "instance-1"
	//
	// Because both unmarshal into the same struct, item config should override.
	if got, want := adp.Spec.Foo, "item-foo"; got != want {
		t.Fatalf("Spec.Foo = %q, want %q (item config should override adapter config)", got, want)
	}
	if got, want := adp.Spec.Label, "instance-1"; got != want {
		t.Fatalf("Spec.Label = %q, want %q", got, want)
	}

	// --- Dependency injection into struct field ---
	if adp.ListerProvider == nil {
		t.Fatalf("ListerProvider is nil; dependency injection failed")
	}

	// Ensure the injected type is our ListerAdp.
	lister, ok := adp.ListerProvider.(*ListerAdp)
	if !ok {
		t.Fatalf("ListerProvider has type %T, want *ListerAdp", adp.ListerProvider)
	}

	// And that its config was loaded from lister-adp.json.
	if got, want := lister.Note, "dummy provider config"; got != want {
		t.Fatalf("ListerAdp.Note = %q, want %q", got, want)
	}

	// --- Behavior of the injected lister ---
	gotList, err := adp.ListerProvider.List(context.Background())
	if err != nil {
		t.Fatalf("ListerProvider.List error: %v", err)
	}
	wantList := []string{"one", "two"}
	if !reflect.DeepEqual(gotList, wantList) {
		t.Fatalf("ListerProvider.List() = %#v, want %#v", gotList, wantList)
	}

	// --- Context handling: exact absolute paths ---

	// Adp: item context should win.
	// inst1.json is at testdata/items/inst1.json with:
	//   "context": "ctx-inst1"
	// The code resolves that relative to the file's directory:
	//   Abs("testdata/items/ctx-inst1")
	expectedAdpCtx, err := filepath.Abs(filepath.Join("ctx-inst1"))
	if err != nil {
		t.Fatalf("filepath.Abs for Adp: %v", err)
	}
	if adp.ContextPath != expectedAdpCtx {
		t.Fatalf("Adp.ContextPath = %q, want %q", adp.ContextPath, expectedAdpCtx)
	}

	// ListerAdp: only adapter-level context.
	// lister-adp.json is at testdata/lister-adp.json with:
	//   "context": "ctx-lister"
	// The code resolves that relative to the file's directory:
	//   Abs("testdata/ctx-lister")
	expectedListerCtx, err := filepath.Abs(filepath.Join("testdata", "ctx-lister"))
	if err != nil {
		t.Fatalf("filepath.Abs for ListerAdp: %v", err)
	}
	if lister.ContextPath != expectedListerCtx {
		t.Fatalf("ListerAdp.ContextPath = %q, want %q", lister.ContextPath, expectedListerCtx)
	}
}
