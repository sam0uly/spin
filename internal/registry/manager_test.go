package registry

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestManager returns a Manager whose CacheDir is a fresh temp
// dir. The caller owns the dir (t.TempDir auto-cleans).
func newTestManager(t *testing.T) Manager {
	t.Helper()
	dir := t.TempDir()
	return Manager{CacheDir: dir}
}

// writeRegistryFixture creates a minimal valid registry dir at
// root/registry.toml + root/templates/<id>.toml. Used to seed
// `spin registry add <path>` happy-path tests.
func writeRegistryFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	regTOML := `id = "fixture"
name = "Fixture Registry"
`
	if err := os.WriteFile(filepath.Join(root, "registry.toml"), []byte(regTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	tplTOML := `id = "go-api"
name = "Go API"
source = "https://github.com/example/go-api.git"
`
	if err := os.WriteFile(filepath.Join(root, "templates", "go-api.toml"), []byte(tplTOML), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAlias_RejectsBad(t *testing.T) {
	bad := []string{
		"",
		"foo/bar",
		"foo\\bar",
		"foo:bar",
		"foo..bar",
		"-leading-dash",
		"foo bar",
		"foo\nbar",
		"foo\x00bar",
	}
	for _, s := range bad {
		if err := ValidateAlias(s); err == nil {
			t.Errorf("ValidateAlias(%q) accepted; want error", s)
		}
	}
}

func TestValidateAlias_AcceptsGood(t *testing.T) {
	good := []string{
		"official",
		"my-registry",
		"my_registry",
		"local.thing",
		"with123numbers",
	}
	for _, s := range good {
		if err := ValidateAlias(s); err != nil {
			t.Errorf("ValidateAlias(%q) errored: %v", s, err)
		}
	}
}

func TestManager_LoadEmpty(t *testing.T) {
	mgr := newTestManager(t)
	cfg, err := mgr.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Registries) != 0 {
		t.Errorf("fresh manager should have zero registries; got %d", len(cfg.Registries))
	}
	_ = cfg
}

func TestManager_AddLocalRegistry(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	writeRegistryFixture(t, src)

	reg, err := mgr.Add(context.Background(), "local", src, false)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if reg.Alias != "local" {
		t.Errorf("alias = %q, want local", reg.Alias)
	}
	if reg.Kind != KindLocal {
		t.Errorf("kind = %q, want local", reg.Kind)
	}
	if reg.AddedAt == "" {
		t.Error("AddedAt should be set")
	}
	// Symlink (or copy on Windows) should exist at the cache path.
	if _, err := os.Stat(reg.Path); err != nil {
		t.Errorf("cached registry path missing: %v", err)
	}
	// registries.json should now list one entry.
	cfg, err := mgr.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Registries) != 1 {
		t.Errorf("registries.json should have 1 entry; got %d", len(cfg.Registries))
	}
}

func TestManager_AddRejectsBadAlias(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	writeRegistryFixture(t, src)

	if _, err := mgr.Add(context.Background(), "bad/alias", src, false); !errors.Is(err, ErrAliasInvalid) {
		t.Errorf("expected ErrAliasInvalid; got %v", err)
	}
}

func TestManager_AddRefusesDuplicateWithoutForce(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	writeRegistryFixture(t, src)

	if _, err := mgr.Add(context.Background(), "dup", src, false); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if _, err := mgr.Add(context.Background(), "dup", src, false); !errors.Is(err, ErrAliasExists) {
		t.Errorf("second Add: expected ErrAliasExists; got %v", err)
	}
	// --force should succeed and rewrite the cache.
	if _, err := mgr.Add(context.Background(), "dup", src, true); err != nil {
		t.Errorf("Add with --force: %v", err)
	}
}

func TestManager_AddRollsBackOnMissingRegistryToml(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	// src exists but has no registry.toml -- NOT a registry
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := mgr.Add(context.Background(), "notareg", src, false)
	if err == nil {
		t.Fatal("Add should error on missing registry.toml")
	}
	// The cache slot should not exist (rollback).
	cachePath := filepath.Join(mgr.RegistriesDir(), "notareg")
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Errorf("cache should be rolled back; stat err=%v", err)
	}
	// registries.json should not have the entry.
	cfg, _ := mgr.Load(context.Background())
	if len(cfg.Registries) != 0 {
		t.Errorf("registries.json should be empty; got %+v", cfg.Registries)
	}
}

func TestManager_AddRollsBackOnMissingTemplatesDir(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	// registry.toml exists, templates/ does not.
	if err := os.WriteFile(filepath.Join(src, "registry.toml"), []byte("id=\"x\"\nname=\"y\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := mgr.Add(context.Background(), "notareg2", src, false)
	if err == nil {
		t.Fatal("Add should error on missing templates/")
	}
	cachePath := filepath.Join(mgr.RegistriesDir(), "notareg2")
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Errorf("cache should be rolled back; stat err=%v", err)
	}
}

func TestManager_GetReturnsFalseForUnknown(t *testing.T) {
	mgr := newTestManager(t)
	if _, ok := mgr.Get(context.Background(), "nope"); ok {
		t.Error("Get should return false for unknown alias")
	}
}

func TestManager_RemoveDeletesCacheAndEntry(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	writeRegistryFixture(t, src)
	reg, err := mgr.Add(context.Background(), "doomed", src, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.Remove(context.Background(), "doomed", nil, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(reg.Path); !os.IsNotExist(err) {
		t.Errorf("cache should be gone; stat err=%v", err)
	}
	if _, ok := mgr.Get(context.Background(), "doomed"); ok {
		t.Error("alias should be gone from registries.json")
	}
}

func TestManager_RemoveKeepsSiblingRegistryCache(t *testing.T) {
	mgr := newTestManager(t)
	srcA := t.TempDir()
	writeRegistryFixture(t, srcA)
	srcB := t.TempDir()
	writeRegistryFixture(t, srcB)
	first, err := mgr.Add(context.Background(), "first", srcA, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := mgr.Add(context.Background(), "second", srcB, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.Remove(context.Background(), "first", nil, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(first.Path); !os.IsNotExist(err) {
		t.Errorf("removed registry cache should be gone; stat err=%v", err)
	}
	if _, err := os.Stat(second.Path); err != nil {
		t.Errorf("sibling registry cache must survive: %v", err)
	}
	if _, ok := mgr.Get(context.Background(), "second"); !ok {
		t.Error("sibling alias should remain registered")
	}
}

func TestManager_RemoveUnknownAliasIsError(t *testing.T) {
	mgr := newTestManager(t)
	err := mgr.Remove(context.Background(), "ghost", nil, false)
	if !errors.Is(err, ErrRegistryMissing) {
		t.Errorf("expected ErrRegistryMissing; got %v", err)
	}
}

func TestManager_RemoveRefusesWhenPinsDepend(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	writeRegistryFixture(t, src)
	if _, err := mgr.Add(context.Background(), "official", src, false); err != nil {
		t.Fatal(err)
	}
	pins := []Pinned{
		{Name: "go-api", Source: "https://github.com/example/go-api.git"},
	}
	// Without --purge-pinned: refuses.
	if err := mgr.Remove(context.Background(), "official", pins, false); err == nil {
		t.Fatal("Remove should refuse when pins depend on registry")
	}
	// Registry should still be present after refused remove.
	if _, ok := mgr.Get(context.Background(), "official"); !ok {
		t.Error("registry should still be registered after refused remove")
	}
	// With --purge-pinned: succeeds.
	if err := mgr.Remove(context.Background(), "official", pins, true); err != nil {
		t.Errorf("Remove with --purge-pinned: %v", err)
	}
	if _, ok := mgr.Get(context.Background(), "official"); ok {
		t.Error("registry should be gone after purge-pinned remove")
	}
}

func TestManager_RefreshLocalIsNoOp(t *testing.T) {
	mgr := newTestManager(t)
	src := t.TempDir()
	writeRegistryFixture(t, src)
	reg, err := mgr.Add(context.Background(), "local", src, false)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := mgr.Refresh(context.Background(), "local")
	if err != nil {
		t.Fatalf("Refresh on local: %v", err)
	}
	if out.LastUpdated != "" {
		t.Errorf("local refresh should not stamp LastUpdated; got %q", out.LastUpdated)
	}
	if out.Alias != reg.Alias {
		t.Errorf("returned alias = %q; want %q", out.Alias, reg.Alias)
	}
}

func TestManager_RefreshUnknownAliasIsError(t *testing.T) {
	mgr := newTestManager(t)
	if _, _, err := mgr.Refresh(context.Background(), "nope"); !errors.Is(err, ErrRegistryMissing) {
		t.Errorf("expected ErrRegistryMissing; got %v", err)
	}
}

func TestManager_WriteRegistriesAtomic(t *testing.T) {
	mgr := newTestManager(t)
	cfg := RegistriesConfig{Registries: []Registry{
		{Alias: "a", Source: "/tmp/a", Kind: KindLocal, Path: "/tmp/a-cache"},
	}}
	if err := mgr.writeRegistries(cfg); err != nil {
		t.Fatalf("writeRegistries: %v", err)
	}
	// File exists, parses cleanly, contains the entry we wrote.
	b, err := os.ReadFile(mgr.RegistriesPath())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var roundtrip RegistriesConfig
	if err := json.Unmarshal(b, &roundtrip); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(roundtrip.Registries) != 1 || roundtrip.Registries[0].Alias != "a" {
		t.Errorf("roundtrip lost data: %+v", roundtrip)
	}
}

func TestManager_FindDependentPinsByPath(t *testing.T) {
	mgr := newTestManager(t)
	reg := Registry{Alias: "x", Path: "/cache/registries/x", Kind: KindLocal}
	pins := []Pinned{
		{Name: "ok", LocalPath: "/elsewhere/templates/ok"},              // outside
		{Name: "gone", LocalPath: "/cache/registries/x/templates/gone"}, // inside
	}
	deps := mgr.findDependentPins(reg, pins)
	if len(deps) != 1 || deps[0].Name != "gone" {
		t.Errorf("expected 1 dependent pin (gone); got %+v", deps)
	}
}

func TestManager_FindDependentPinsByTemplateSource(t *testing.T) {
	mgr := newTestManager(t)
	// Build a real registry with a template whose `source` URL is
	// what we'll pin against.
	root := t.TempDir()
	writeRegistryFixture(t, root)
	reg, err := mgr.Add(context.Background(), "bySrc", root, false)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	pins := []Pinned{
		{Name: "go-api", Source: "https://github.com/example/go-api.git"}, // matches fixture
		{Name: "other", Source: "https://example.com/y.git"},
	}
	deps := mgr.findDependentPins(reg, pins)
	if len(deps) != 1 || deps[0].Name != "go-api" {
		t.Errorf("expected 1 dependent pin (go-api); got %+v", deps)
	}
}

func TestManager_AddEmptySourceIsError(t *testing.T) {
	mgr := newTestManager(t)
	if _, err := mgr.Add(context.Background(), "empty", "", false); err == nil {
		t.Error("Add with empty source should error")
	}
}

func TestManager_AddNonExistentLocalSource(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.Add(context.Background(), "missing", "/no/such/path", false)
	if err == nil {
		t.Fatal("Add with non-existent local source should error")
	}
	if strings.Contains(err.Error(), "registry:") || strings.Contains(err.Error(), "add:") {
		t.Errorf("error should be flat, without 'registry:' or 'add:' prefixes; got %v", err)
	}
}

func TestManager_AddNonExistentGitSource(t *testing.T) {
	mgr := newTestManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := mgr.Add(ctx, "missing", "https://github.com/spin-org/does-not-exist-12345.git", false)
	if err == nil {
		t.Fatal("Add with non-existent git source should error")
	}
}

// TestManager_AddLocalNonDirectoryErrors confirms a regular file as
// the source is rejected, matching the existing pin-side behaviour.
func TestManager_AddLocalNonDirectoryErrors(t *testing.T) {
	mgr := newTestManager(t)
	f := filepath.Join(t.TempDir(), "afile.txt")
	if err := os.WriteFile(f, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Add(context.Background(), "file", f, false); err == nil {
		t.Error("Add with file source should error")
	} else if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected 'not a directory' in error; got %v", err)
	}
}

// withFixtureOfficialURL points DefaultRegistryURL at a local registry
// fixture for the duration of the test, so Bootstrap never touches the
// network. It returns the fixture path.
func withFixtureOfficialURL(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	writeRegistryFixture(t, src)
	oldURL := DefaultRegistryURL
	DefaultRegistryURL = src
	t.Cleanup(func() { DefaultRegistryURL = oldURL })
	return src
}

func TestManager_BootstrapFirstRun(t *testing.T) {
	src := withFixtureOfficialURL(t)
	mgr := newTestManager(t)

	did, err := mgr.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if !did {
		t.Fatal("Bootstrap reported did=false on first run")
	}
	reg, ok := mgr.Get(context.Background(), "official")
	if !ok {
		t.Fatal("official registry should be registered after first-run bootstrap")
	}
	if reg.Source != src {
		t.Errorf("Source = %q, want %q", reg.Source, src)
	}
	if reg.Kind != KindLocal {
		t.Errorf("Kind = %q, want local (fixture symlink)", reg.Kind)
	}
}

func TestManager_BootstrapIdempotent(t *testing.T) {
	withFixtureOfficialURL(t)
	mgr := newTestManager(t)

	if _, err := mgr.Bootstrap(context.Background()); err != nil {
		t.Fatalf("first Bootstrap: %v", err)
	}
	// Second call: registries.json now exists, so Bootstrap must be a
	// no-op -- not a re-clone and not ErrAliasExists.
	did, err := mgr.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("second Bootstrap: %v", err)
	}
	if did {
		t.Error("Bootstrap reported did=true when registries.json already exists")
	}
}

func TestManager_BootstrapNoopWhenConfigured(t *testing.T) {
	mgr := newTestManager(t)
	if err := os.WriteFile(mgr.RegistriesPath(),
		[]byte(`{"registries":[{"alias":"custom","source":"/tmp/x","kind":"local"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	did, err := mgr.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if did {
		t.Error("Bootstrap reported did=true for an existing registries.json")
	}
	if _, ok := mgr.Get(context.Background(), "official"); ok {
		t.Error("official registry must not be added when registries.json already exists")
	}
}

func TestManager_BootstrapNoopOnCorruptFile(t *testing.T) {
	mgr := newTestManager(t)
	if err := os.WriteFile(mgr.RegistriesPath(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	did, err := mgr.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if did {
		t.Error("Bootstrap must not overwrite a corrupt registries.json")
	}
	if b, err := os.ReadFile(mgr.RegistriesPath()); err != nil || string(b) != "{not json" {
		t.Errorf("corrupt file was modified: %q, err=%v", b, err)
	}
}

func TestManager_BootstrapNoopOnEmptyFile(t *testing.T) {
	mgr := newTestManager(t)
	if err := os.WriteFile(mgr.RegistriesPath(), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	did, err := mgr.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if did {
		t.Error("Bootstrap reported did=true for an empty registries.json")
	}
}

func TestManager_BootstrapHonoursContext(t *testing.T) {
	mgr := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	did, err := mgr.Bootstrap(ctx)
	if err == nil {
		t.Fatal("Bootstrap should return the cancellation error")
	}
	if did {
		t.Error("Bootstrap reported did=true on cancelled context")
	}
}

func TestManager_BootstrapDefersToLockHolder(t *testing.T) {
	mgr := newTestManager(t)
	if err := os.MkdirAll(mgr.CacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(mgr.CacheDir, ".bootstrap.lock")
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	did, err := mgr.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if did {
		t.Error("Bootstrap reported did=true while another process holds the lock")
	}
	if _, ok := mgr.Get(context.Background(), "official"); ok {
		t.Error("official registry must not be added while another process holds the lock")
	}
}

func TestManager_BootstrapReclaimsStaleLock(t *testing.T) {
	withFixtureOfficialURL(t)
	mgr := newTestManager(t)
	if err := os.MkdirAll(mgr.CacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(mgr.CacheDir, ".bootstrap.lock")
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-bootstrapLockTTL - time.Minute)
	if err := os.Chtimes(lockPath, stale, stale); err != nil {
		t.Fatal(err)
	}
	did, err := mgr.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if !did {
		t.Error("Bootstrap should reclaim a stale lock and register the official registry")
	}
	if _, ok := mgr.Get(context.Background(), "official"); !ok {
		t.Error("official registry should be registered after reclaiming a stale lock")
	}
}
