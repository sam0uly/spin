package registry

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsShorthand(t *testing.T) {
	cases := []struct {
		spec string
		want bool
	}{
		{"official/go-api", true},
		{"my/local", true},
		{"a/b", true},
		{"foo", false},                       // no slash
		{"/abs/path", false},                 // local path
		{"./rel", false},                     // local path
		{"~/home", false},                    // local path
		{"https://example.com/x.git", false}, // git URL
		{"git@host:foo/bar", false},          // git URL
		{"a/b/c", false},                     // more than one slash
		{"/leading", false},                  // local path
		{"trailing/", false},                 // empty id
		{"", false},
	}
	for _, c := range cases {
		got := IsShorthand(c.spec)
		if got != c.want {
			t.Errorf("IsShorthand(%q) = %v; want %v", c.spec, got, c.want)
		}
	}
}

func TestManager_BuildReturnsEntries(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	writeRegistryFixture(t, src)
	if _, err := mgr.Add(context.Background(), "official", src, false); err != nil {
		t.Fatal(err)
	}
	idx, skip, err := mgr.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(skip) != 0 {
		t.Errorf("expected zero skip; got %+v", skip)
	}
	if len(idx.entries) != 1 {
		t.Fatalf("expected 1 entry; got %d: %+v", len(idx.entries), idx.entries)
	}
	e := idx.entries[0]
	if e.Alias != "official" || e.ID != "go-api" {
		t.Errorf("entry mismatch: %+v", e)
	}
	if e.Name != "Go API" {
		t.Errorf("name = %q, want Go API", e.Name)
	}
}

func TestManager_BuildSkipsInvalidTemplates(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	writeRegistryFixture(t, src)
	// Add a template that fails validation: id missing.
	if err := os.WriteFile(filepath.Join(src, "templates", "bad.toml"),
		[]byte("name = \"Bad\"\nsource = \"https://x.com/y.git\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Add another: id mismatch with file name.
	if err := os.WriteFile(filepath.Join(src, "templates", "actual-name.toml"),
		[]byte("id = \"different-name\"\nname = \"Mismatch\"\nsource = \"https://x.com/y.git\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Add(context.Background(), "official", src, false); err != nil {
		t.Fatal(err)
	}
	idx, skip, err := mgr.Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if skip["official"] != 2 {
		t.Errorf("expected 2 skipped under 'official'; got %d", skip["official"])
	}
	if len(idx.entries) != 1 {
		t.Errorf("expected 1 valid entry; got %d", len(idx.entries))
	}
}

func TestIndex_SearchByID(t *testing.T) {
	idx := &Index{entries: []TemplateEntry{
		{Alias: "official", ID: "go-api", Name: "Go API", Description: "Go REST starter"},
		{Alias: "official", ID: "rust-cli", Name: "Rust CLI", Description: "Rust starter"},
		{Alias: "community", ID: "go-tui", Name: "Go TUI", Description: "Bubbletea starter"},
	}}
	got := idx.Search("go", 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 matches for 'go'; got %d: %+v", len(got), got)
	}
	// go-api should rank above go-tui (substring of id vs substring of name).
	if got[0].ID != "go-api" {
		t.Errorf("first result should be go-api; got %s", got[0].ID)
	}
}

func TestIndex_SearchEmptyQuery(t *testing.T) {
	idx := &Index{entries: []TemplateEntry{
		{Alias: "a", ID: "x", Name: "X"},
		{Alias: "b", ID: "y", Name: "Y"},
	}}
	got := idx.Search("", 0)
	if len(got) != 2 {
		t.Errorf("empty query should return all; got %d", len(got))
	}
}

func TestIndex_SearchLimit(t *testing.T) {
	idx := &Index{entries: []TemplateEntry{
		{Alias: "a", ID: "1"}, {Alias: "a", ID: "2"}, {Alias: "a", ID: "3"},
	}}
	got := idx.Search("", 2)
	if len(got) != 2 {
		t.Errorf("limit=2 should return 2; got %d", len(got))
	}
}

func TestManager_ValidateReportsIssues(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	writeRegistryFixture(t, src)
	// Drop a malformed template.
	if err := os.WriteFile(filepath.Join(src, "templates", "broken.toml"), []byte("not = \" = valid toml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Add(context.Background(), "off", src, false); err != nil {
		t.Fatal(err)
	}
	issues := mgr.Validate(context.Background(), "off")
	if len(issues) == 0 {
		t.Error("expected Validate to report at least one issue (broken.toml)")
	}
}

func TestManager_ResolveShorthandHappyPath(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	writeRegistryFixture(t, src)
	if _, err := mgr.Add(context.Background(), "official", src, false); err != nil {
		t.Fatal(err)
	}
	res, err := mgr.ResolveShorthand(context.Background(), "official/go-api")
	if err != nil {
		t.Fatalf("ResolveShorthand: %v", err)
	}
	if res.Alias != "official" || res.ID != "go-api" {
		t.Errorf("alias/id mismatch: %+v", res)
	}
	if res.Source != "https://github.com/example/go-api.git" {
		t.Errorf("source = %q; want https://github.com/example/go-api.git", res.Source)
	}
}

func TestManager_ResolveShorthandUnknownAlias(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.ResolveShorthand(context.Background(), "ghost/nope")
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected 'not registered' error; got %v", err)
	}
}

func TestManager_ResolveShorthandOfficialNotRegistered(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.ResolveShorthand(context.Background(), "official/go-api")
	if err == nil {
		t.Fatal("expected error for unregistered official alias")
	}
	var notRegistered AliasNotRegisteredError
	if !errors.As(err, &notRegistered) {
		t.Fatalf("expected AliasNotRegisteredError; got %T: %v", err, err)
	}
	if notRegistered.Alias != "official" {
		t.Errorf("alias = %q; want official", notRegistered.Alias)
	}
	// The registry layer must stay transport-agnostic: the failure
	// is structured data, and CLI hints belong to the cmd layer.
	if strings.Contains(err.Error(), "spin registry add") {
		t.Errorf("registry error must not contain CLI hints; got %v", err)
	}
}

func TestManager_ResolveShorthandUnknownID(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	writeRegistryFixture(t, src)
	if _, err := mgr.Add(context.Background(), "official", src, false); err != nil {
		t.Fatal(err)
	}
	_, err := mgr.ResolveShorthand(context.Background(), "official/nope")
	if err == nil || !strings.Contains(err.Error(), "not in registry") {
		t.Errorf("expected 'not in registry' error; got %v", err)
	}
}

func TestManager_ResolveShorthandNotShorthand(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.ResolveShorthand(context.Background(), "not a shorthand")
	if err == nil || !strings.Contains(err.Error(), "<alias>/<id>") {
		t.Errorf("expected '<alias>/<id>' error; got %v", err)
	}
}

// TestManager_ResolveShorthandChainsOnce verifies the resolver follows a template's `source` shorthand one level.
func TestManager_ResolveShorthandChainsOnce(t *testing.T) {
	mgr := newTestManager(t)
	srcA := t.TempDir()
	writeRegistryFixture(t, srcA)
	// Replace go-api's source with a shorthand that points at
	// another registry's template.
	nested := `id = "go-api"
name = "Go API"
source = "secondary/go-api-renamed"
`
	if err := os.WriteFile(filepath.Join(srcA, "templates", "go-api.toml"), []byte(nested), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Add(context.Background(), "official", srcA, false); err != nil {
		t.Fatal(err)
	}

	// Build a second registry whose template's source is the
	// real git URL.
	srcB := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcB, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcB, "registry.toml"),
		[]byte("id=\"secondary\"\nname=\"S\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcB, "templates", "go-api-renamed.toml"),
		[]byte("id=\"go-api-renamed\"\nname=\"R\"\nsource=\"https://github.com/example/go-api.git\"\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Add(context.Background(), "secondary", srcB, false); err != nil {
		t.Fatal(err)
	}

	res, err := mgr.ResolveShorthand(context.Background(), "official/go-api")
	if err != nil {
		t.Fatalf("ResolveShorthand chain: %v", err)
	}
	if res.Source != "https://github.com/example/go-api.git" {
		t.Errorf("chained source = %q; want https://github.com/example/go-api.git", res.Source)
	}
	if res.Alias != "secondary" {
		t.Errorf("chained alias = %q; want secondary", res.Alias)
	}
}
