package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sam0uly/spin/internal/registry"
)

// writeMiniRegistry creates a minimal valid registry at root (registry.toml + templates/<id>.
func writeMiniRegistry(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	regTOML := "id = \"fixture\"\nname = \"Fixture Registry\"\n"
	if err := os.WriteFile(filepath.Join(root, "registry.toml"), []byte(regTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	tplTOML := "id = \"go-api\"\nname = \"Go API\"\nsource = \"https://github.com/example/go-api.git\"\n"
	if err := os.WriteFile(filepath.Join(root, "templates", "go-api.toml"), []byte(tplTOML), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestMaybeBootstrapOfficial_FirstRun adds the official registry when registries.json does not exist.
func TestMaybeBootstrapOfficial_FirstRun(t *testing.T) {
	src := t.TempDir()
	writeMiniRegistry(t, src)
	oldURL := registry.DefaultRegistryURL
	registry.DefaultRegistryURL = src
	t.Cleanup(func() { registry.DefaultRegistryURL = oldURL })

	mgr := registry.NewManager().SetCacheDir(t.TempDir())
	maybeBootstrapOfficial(context.Background(), &mgr)
	if _, ok := mgr.Get(context.Background(), "official"); !ok {
		t.Fatal("maybeBootstrapOfficial should add the official registry on first run")
	}
}

// TestAnnotateShorthandError verifies the CLI enrichment: the official alias gets a re-add hint, while other.
func TestAnnotateShorthandError(t *testing.T) {
	enriched, ok := annotateShorthandError(registry.AliasNotRegisteredError{Alias: "official"})
	if !ok {
		t.Fatal("expected official alias error to be enriched")
	}
	if !strings.Contains(enriched.Error(), "spin registry add official") {
		t.Errorf("expected re-add hint in enriched error; got %v", enriched)
	}

	ghost := registry.AliasNotRegisteredError{Alias: "ghost"}
	unchanged, ok := annotateShorthandError(ghost)
	if ok {
		t.Errorf("non-official alias must not be enriched; got %v", unchanged)
	}
	if unchanged != ghost {
		t.Errorf("error must pass through unchanged; got %v", unchanged)
	}

	if _, ok := annotateShorthandError(nil); ok {
		t.Error("nil error must not be enriched")
	}
}

// TestMaybeBootstrapOfficial_NoopWhenConfigured verifies the helper does not touch the network or mutate registries.
func TestMaybeBootstrapOfficial_NoopWhenConfigured(t *testing.T) {
	mgr := registry.NewManager().SetCacheDir(t.TempDir())
	if err := os.WriteFile(mgr.RegistriesPath(), []byte(`{"registries":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	maybeBootstrapOfficial(context.Background(), &mgr)
	if _, ok := mgr.Get(context.Background(), "official"); ok {
		t.Fatal("maybeBootstrapOfficial must not re-add the official registry when registries.json exists")
	}
}
