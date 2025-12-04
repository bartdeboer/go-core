package core_test

import (
	"context"
	"reflect"
	"testing"

	core "github.com/bartdeboer/go-core"
)

// ListerAdp implements core.Lister and also supports config loading.
type ListerAdp struct {
	Note string `json:"note"`
}

func (l *ListerAdp) List(ctx context.Context) ([]string, error) {
	return []string{"one", "two"}, nil
}

// Make ListerAdp configurable so lister-adp.json is actually used.
func (l *ListerAdp) ConfigPtr() any {
	return l
}

// Adp is the main adapter under test.
type Adp struct {
	ItemSpec struct {
		Name  string `json:"name"`
		Foo   string `json:"foo"`
		Label string `json:"label"`
	}

	Spec struct {
		Label string `json:"label"`
		Foo   string `json:"foo"`
	}

	// Injected from dependencies in JSON:
	// "dependencies": { "ListerProvider": { "adapter": "lister-adp" } }
	ListerProvider core.Lister `core:"required"`
}

// ConfigPtr makes Adp implement core.Configurable.
func (a *Adp) ConfigPtr() any {
	return &a.Spec
}

// ItemConfigPtr makes Adp implement core.ItemConfigurable.
func (a *Adp) ItemConfigPtr(name string) any {
	// capture the resolved item name
	a.ItemSpec.Name = name
	return &a.ItemSpec
}

func TestAdapter_DependencyInjectionAndConfigLoading(t *testing.T) {
	// Use configs from ./testdata.
	if _, err := core.NewSearchMap("testdata"); err != nil {
		t.Fatalf("NewSearchMap: %v", err)
	}

	// Register adapters.
	core.Register("lister-adp", func() core.Adapter { return &ListerAdp{} })
	core.Register("adp", func() core.Adapter { return &Adp{} })

	// Create an instance of "adp" using the item config "items/inst1".
	adp, err := core.NewAdapterAs[*Adp]("adp", "items/inst1")
	if err != nil {
		t.Fatalf("NewAdapterAs(adp): %v", err)
	}

	// --- Adapter-level config (adp.json) ---
	if got, want := adp.Spec.Foo, "global-foo"; got != want {
		t.Fatalf("Spec.Foo = %q, want %q", got, want)
	}

	// --- Item-level config (items/inst1.json) ---
	if got, want := adp.ItemSpec.Foo, "item-foo"; got != want {
		t.Fatalf("ItemSpec.Foo = %q, want %q", got, want)
	}
	if got, want := adp.ItemSpec.Label, "instance-1"; got != want {
		t.Fatalf("ItemSpec.Label = %q, want %q", got, want)
	}
	// Name should come from itemMeta.Name ("inst1").
	if got, want := adp.ItemSpec.Name, "inst1"; got != want {
		t.Fatalf("ItemSpec.Name = %q, want %q", got, want)
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
}
