package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sam0uly/spin/internal/registry"
)

// withEmptyPinned sets XDG_CONFIG_HOME to a temp dir for the duration of the test, so the registry client.
func withEmptyPinned(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return filepath.Join(dir, "spin")
}

// seedPinned writes pinned records to the test cache and returns
// the path. Used by tests that need a non-empty pinned list.
func seedPinned(t *testing.T, cache string, pins ...registry.Pinned) {
	t.Helper()
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(pins, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "pinned.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// readPinned parses the test cache's pinned.json.
func readPinned(t *testing.T, cache string) []registry.Pinned {
	t.Helper()
	var out []registry.Pinned
	b, err := os.ReadFile(filepath.Join(cache, "pinned.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	if len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestRemove_UnknownNameIsError verifies `spin remove foo` errors when "foo" isn't pinned.
func TestRemove_UnknownNameIsError(t *testing.T) {
	withEmptyPinned(t)
	out, exitCode := runSpinExit(t, "remove", "nonexistent")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit; got 0\n%s", out)
	}
	if !bytes.Contains(out, []byte("nonexistent")) {
		t.Errorf("error should mention the bad name; got:\n%s", out)
	}
	if !bytes.Contains(out, []byte("spin list")) {
		t.Errorf("error should suggest `spin list`; got:\n%s", out)
	}
}

// TestRemove_MarksRemoved verifies the default `spin remove` soft-deletes the pin: the record stays in pinned.
func TestRemove_MarksRemoved(t *testing.T) {
	cache := withEmptyPinned(t)
	seedPinned(t, cache,
		registry.Pinned{Name: "keepme", LocalPath: "/tmp/keepme"},
		registry.Pinned{Name: "byebye", LocalPath: "/tmp/byebye"},
	)
	out, exitCode := runSpinExit(t, "remove", "byebye")
	if exitCode != 0 {
		t.Fatalf("expected exit 0; got %d\n%s", exitCode, out)
	}
	pins := readPinned(t, cache)
	var found *registry.Pinned
	for i := range pins {
		if pins[i].Name == "byebye" {
			found = &pins[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("byebye should still be in pinned.json (soft-deleted); got: %+v", pins)
	}
	if !found.Removed {
		t.Errorf("byebye should be marked Removed=true; got %+v", found)
	}
}

// TestRemove_PurgeDeletesCache is the regression test for the `--purge` bug: removing a pin and then purging it.
func TestRemove_PurgeDeletesCache(t *testing.T) {
	cache := withEmptyPinned(t)
	pinDir := filepath.Join(t.TempDir(), "toremove")
	if err := os.MkdirAll(pinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seedPinned(t, cache, registry.Pinned{Name: "toremove", LocalPath: pinDir})

	// Step 1: remove WITHOUT --purge. Cache should remain; pin
	// should be marked Removed so --purge can still find it.
	if out, exit := runSpinExit(t, "remove", "toremove"); exit != 0 {
		t.Fatalf("first remove: exit=%d\n%s", exit, out)
	}
	if _, err := os.Stat(pinDir); err != nil {
		t.Fatalf("cache should remain after plain `spin remove`; stat err=%v", err)
	}
	pins := readPinned(t, cache)
	var found *registry.Pinned
	for i := range pins {
		if pins[i].Name == "toremove" {
			found = &pins[i]
		}
	}
	if found == nil || !found.Removed {
		t.Fatalf("pin should still be in pinned.json with Removed=true; got: %+v", pins)
	}

	// Step 2: purge. This is the step that used to no-op silently.
	out, exit := runSpinExit(t, "remove", "toremove", "--purge")
	if exit != 0 {
		t.Fatalf("--purge: exit=%d\n%s", exit, out)
	}
	if _, err := os.Stat(pinDir); !os.IsNotExist(err) {
		t.Errorf("cache should be deleted after `spin remove --purge`; stat err=%v", err)
	}
	pins = readPinned(t, cache)
	for _, p := range pins {
		if p.Name == "toremove" {
			t.Errorf("pin should be gone after --purge; got: %+v", p)
		}
	}
}

// TestList_JSONOutput verifies --json emits a valid JSON array
// of pinnedRow, one per pin, with no styling/escape codes.
func TestList_JSONOutput(t *testing.T) {
	cache := withEmptyPinned(t)
	pinDir := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(pinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pinDir, "spin.toml"),
		[]byte("name = \"demo\"\ndescription = \"a demo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedPinned(t, cache, registry.Pinned{
		Name:      "demo",
		Version:   "v1.0.0",
		Source:    "https://example.com/demo.git",
		LocalPath: pinDir,
		PinnedAt:  "2026-06-14T00:00:00Z",
	})

	out := runSpin(t, "list", "--json")
	var rows []pinnedRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Name != "demo" {
		t.Errorf("name = %q, want demo", rows[0].Name)
	}
	if rows[0].Description != "a demo" {
		t.Errorf("description = %q, want 'a demo'", rows[0].Description)
	}
	if rows[0].Version != "v1.0.0" {
		t.Errorf("version = %q, want v1.0.0", rows[0].Version)
	}
}

// TestList_JSONOutputEmpty verifies the empty-list JSON output is a valid empty array, not an error or "no.
func TestList_JSONOutputEmpty(t *testing.T) {
	withEmptyPinned(t)
	out := runSpin(t, "list", "--json")
	trimmed := strings.TrimSpace(string(out))
	if trimmed != "[]" {
		t.Errorf("empty list --json should be '[]', got: %q", trimmed)
	}
}

// TestList_DefaultIsTable verifies the default table view shows each pin's name and version.
func TestList_DefaultIsTable(t *testing.T) {
	cache := withEmptyPinned(t)
	seedPinned(t, cache, registry.Pinned{
		Name: "mything", Version: "abc123", LocalPath: "/tmp/mything",
	})
	out := runSpin(t, "list")
	if !bytes.Contains(out, []byte("mything")) {
		t.Errorf("default `spin list` should mention pin name; got:\n%s", out)
	}
	if !bytes.Contains(out, []byte("abc123")) {
		t.Errorf("default `spin list` should mention version; got:\n%s", out)
	}
	if bytes.Contains(out, []byte("[")) && bytes.Contains(out, []byte("{")) {
		t.Errorf("default `spin list` should not emit JSON; got:\n%s", out)
	}
}

// TestList_HidesRemovedByDefault verifies soft-deleted pins stay hidden from the default listing.
func TestList_HidesRemovedByDefault(t *testing.T) {
	cache := withEmptyPinned(t)
	seedPinned(t, cache,
		registry.Pinned{Name: "active", LocalPath: "/tmp/active"},
		registry.Pinned{Name: "gone", LocalPath: "/tmp/gone", Removed: true},
	)
	out := runSpin(t, "list")
	if !bytes.Contains(out, []byte("active")) {
		t.Errorf("default list should show active pin; got:\n%s", out)
	}
	if bytes.Contains(out, []byte("gone")) {
		t.Errorf("default list should NOT show removed pin; got:\n%s", out)
	}
}

// TestList_AllShowsRemoved verifies `spin list --all` surfaces soft-deleted pins with the "(removed)" marker, so.
func TestList_AllShowsRemoved(t *testing.T) {
	cache := withEmptyPinned(t)
	seedPinned(t, cache,
		registry.Pinned{Name: "active", LocalPath: "/tmp/active"},
		registry.Pinned{Name: "gone", LocalPath: "/tmp/gone", Removed: true},
	)
	out := runSpin(t, "list", "--all")
	if !bytes.Contains(out, []byte("active")) {
		t.Errorf("--all list should show active pin; got:\n%s", out)
	}
	if !bytes.Contains(out, []byte("gone")) {
		t.Errorf("--all list should show removed pin; got:\n%s", out)
	}
	if !bytes.Contains(out, []byte("(removed)")) {
		t.Errorf("--all list should tag the removed pin with '(removed)'; got:\n%s", out)
	}

	// JSON path also exposes the Removed flag.
	jsonOut := runSpin(t, "list", "--all", "--json")
	var rows []pinnedRow
	if err := json.Unmarshal(jsonOut, &rows); err != nil {
		t.Fatalf("--all --json is not valid JSON: %v\n%s", err, jsonOut)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2; got:\n%s", len(rows), jsonOut)
	}
	removedCount := 0
	for _, r := range rows {
		if r.Removed {
			removedCount++
		}
	}
	if removedCount != 1 {
		t.Errorf("expected exactly 1 row with removed=true; got %d", removedCount)
	}
}

// pinFixture creates a real `spin init go-cli` template in a fresh dir and pins it via `spin add`.
func pinFixture(t *testing.T, name string) string {
	t.Helper()
	tplParent := t.TempDir()
	initOut, initExit := runSpinWithDir(t, tplParent, "init", name)
	if initExit != 0 {
		t.Fatalf("spin init %s: exit=%d\n%s", name, initExit, initOut)
	}
	tplPath := filepath.Join(tplParent, name)

	addOut, addExit := runSpinExit(t, "add", tplPath)
	if addExit != 0 {
		t.Fatalf("spin add %s: exit=%d\n%s", tplPath, addExit, addOut)
	}

	// `spin add` symlinks or copies the template under
	// XDG_CONFIG_HOME/spin/templates/<name>; return that path so
	// the caller can stat it.
	xdg := os.Getenv("XDG_CONFIG_HOME")
	cacheRoot := filepath.Join(xdg, "spin", "templates", name)
	if _, err := os.Stat(cacheRoot); err != nil {
		t.Fatalf("pinned cache dir missing at %s: %v", cacheRoot, err)
	}
	return cacheRoot
}

// TestRemove_FixtureKeepsCache verifies `spin remove <name>` on a real pinned fixture marks the entry removed but.
func TestRemove_FixtureKeepsCache(t *testing.T) {
	cache := withEmptyPinned(t)
	pinDir := pinFixture(t, "go-cli")

	rmOut, rmExit := runSpinExit(t, "remove", "go-cli")
	if rmExit != 0 {
		t.Fatalf("spin remove go-cli: exit=%d\n%s", rmExit, rmOut)
	}
	if !bytes.Contains(rmOut, []byte("cache kept at")) {
		t.Errorf("output should mention cache kept; got:\n%s", rmOut)
	}
	if !bytes.Contains(rmOut, []byte("--purge")) {
		t.Errorf("output should hint at --purge; got:\n%s", rmOut)
	}

	// Cache on disk must still exist.
	if _, err := os.Stat(pinDir); err != nil {
		t.Errorf("cache should remain after plain remove; stat err=%v", err)
	}

	// pinned.json should have the entry with Removed=true.
	pins := readPinned(t, cache)
	if len(pins) != 1 {
		t.Fatalf("pinned.json should still contain 1 entry; got %d: %+v", len(pins), pins)
	}
	if !pins[0].Removed {
		t.Errorf("entry should be marked Removed=true; got %+v", pins[0])
	}

	// Default list must hide the removed pin.
	listOut := runSpin(t, "list")
	if bytes.Contains(listOut, []byte("go-cli")) {
		t.Errorf("default list should hide removed pin; got:\n%s", listOut)
	}

	// `spin new --template <name>` must now fail with the flat
	// "not a local path, git URL, or pinned name" error -- the
	// pin is removed, so the pinned-name lookup no longer finds
	// it.
	newOut, newExit := runSpinExit(t, "new", "demoapp", "go-cli", "--print-params")
	if newExit == 0 {
		t.Fatalf("spin new on removed pin should fail; got exit 0\n%s", newOut)
	}
	if !bytes.Contains(newOut, []byte("not a local path, git URL, or pinned name")) {
		t.Errorf("error should explain the spec is unknown; got:\n%s", newOut)
	}
}

// TestRemove_FixturePurgeClearsEverything is the end-to-end regression test: pin a real fixture, remove it (cache.
func TestRemove_FixturePurgeClearsEverything(t *testing.T) {
	cache := withEmptyPinned(t)
	pinDir := pinFixture(t, "go-cli")

	// Step 1: plain remove -- cache kept, entry marked removed.
	if out, exit := runSpinExit(t, "remove", "go-cli"); exit != 0 {
		t.Fatalf("first remove: exit=%d\n%s", exit, out)
	}
	if _, err := os.Stat(pinDir); err != nil {
		t.Fatalf("cache should remain after plain remove; stat err=%v", err)
	}

	// Step 2: purge -- the previously-broken call.
	out, exit := runSpinExit(t, "remove", "go-cli", "--purge")
	if exit != 0 {
		t.Fatalf("--purge: exit=%d\n%s", exit, out)
	}
	if !bytes.Contains(out, []byte("deleted cache at")) {
		t.Errorf("--purge output should mention deleted cache; got:\n%s", out)
	}
	if _, err := os.Stat(pinDir); !os.IsNotExist(err) {
		t.Errorf("cache should be deleted after --purge; stat err=%v", err)
	}

	pins := readPinned(t, cache)
	for _, p := range pins {
		if p.Name == "go-cli" {
			t.Errorf("pinned.json should no longer contain go-cli; got %+v", p)
		}
	}

	// `spin list --all` is now empty.
	allOut := runSpin(t, "list", "--all", "--json")
	if strings.TrimSpace(string(allOut)) != "[]" {
		t.Errorf("--all should be [] after purge; got:\n%s", allOut)
	}
}
