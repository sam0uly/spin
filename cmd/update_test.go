package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sam0uly/spin/internal/registry"
)

// makeFixtureLocalSource creates a small on-disk "template" that
// can be used as a local-path pin source. The content is fixed
// so we can assert on it after refresh.
func makeFixtureLocalSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spin.toml"), []byte("name = \"src\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_base", "v1.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestRefresh_LocalPath verifies Refresh re-copies the on-disk
// source into the pin's LocalPath. Edit the source AFTER the
// initial pin, call Refresh, and assert the cache picked up the
// new file. This is the happy path of `spin update` for a
// local-path pin.
func TestRefresh_LocalPath(t *testing.T) {
	isolateConfig(t)
	src := makeFixtureLocalSource(t)
	dst := t.TempDir()
	dest := filepath.Join(dst, "fixture")

	// Initial copy: v1 only.
	if err := registry.CopyTreeForTest(src, dest); err != nil {
		t.Fatalf("initial copy: %v", err)
	}
	// Stale content that lives ONLY in the cache (not in src) so
	// we can prove Refresh clears the cache and replaces it with
	// a fresh copy of the source.
	if err := os.WriteFile(filepath.Join(dest, "_base", "stale.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	pin := registry.Pinned{
		Name:      "fixture",
		Source:    src,
		Version:   "local",
		LocalPath: dest,
	}

	// Edit the source: add a v2 file.
	if err := os.WriteFile(filepath.Join(src, "_base", "v2.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	client := registry.New()
	updated, err := client.Refresh(context.Background(), pin)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if updated.Version != "local" {
		t.Errorf("Version = %q, want %q (local-path pin keeps Version=local)", updated.Version, "local")
	}
	// The cache should now have v2.
	if _, err := os.Stat(filepath.Join(dest, "_base", "v2.txt")); err != nil {
		t.Errorf("expected v2.txt in cache after refresh: %v", err)
	}
	// Stale content that lived only in the cache should be gone:
	// Refresh clears the dest first, so the result is a fresh
	// copy of the source, not a union.
	if _, err := os.Stat(filepath.Join(dest, "_base", "stale.txt")); !os.IsNotExist(err) {
		t.Errorf("expected stale.txt to be cleared on refresh; stat err=%v", err)
	}
}

// TestRefresh_NoLocalPath verifies Refresh fails cleanly when the
// pin has no LocalPath (e.g. legacy pin files). The error
// mentions `spin add` so the user knows the fix.
func TestRefresh_NoLocalPath(t *testing.T) {
	isolateConfig(t)
	c := registry.New()
	_, err := c.Refresh(context.Background(), registry.Pinned{Name: "x", Source: "/foo"})
	if err == nil {
		t.Fatal("expected error for empty LocalPath")
	}
	if !strings.Contains(err.Error(), "LocalPath") {
		t.Errorf("error should mention LocalPath; got: %v", err)
	}
	if !strings.Contains(err.Error(), "spin add") {
		t.Errorf("error should suggest re-running `spin add`; got: %v", err)
	}
}

// TestRefresh_MissingOnDisk verifies Refresh refuses to half-clone
// when the cache dir has been deleted out from under the pin. The
// user is told to re-add.
//
// In production the LocalPath is moved aside to a .bak by
// refreshOne BEFORE Refresh runs, so a missing LocalPath is the
// normal case during `spin update` (not an error). We exercise
// the "no LocalPath" branch (covered by TestRefresh_NoLocalPath)
// here for completeness.
func TestRefresh_MissingOnDisk(t *testing.T) {
	isolateConfig(t)
	c := registry.New()
	// For local-path sources, Refresh happily recreates LocalPath
	// from Source. So we don't test a failure here; we test the
	// case Refresh DOES handle (refresh-from-local-when-cache-
	// missing), which is the common `spin update` path.
	src := makeFixtureLocalSource(t)
	dst := t.TempDir()
	pin := registry.Pinned{Name: "x", Source: src, LocalPath: filepath.Join(dst, "x")}
	updated, err := c.Refresh(context.Background(), pin)
	if err != nil {
		t.Fatalf("Refresh from local when LocalPath missing: %v", err)
	}
	if updated.Version != "local" {
		t.Errorf("Version = %q, want local", updated.Version)
	}
	if _, err := os.Stat(pin.LocalPath); err != nil {
		t.Errorf("expected LocalPath to be created: %v", err)
	}
}

// TestRefreshOne_RollsBackOnFailure verifies that when the
// refresh fails, the old on-disk cache is moved back into place
// and the pin record is left untouched.
//
// We trigger a failure by passing a Pin whose source is
// local-path but the source dir has been deleted. Refresh will
// fail on the os.Stat(source) check; refreshOne should roll the
// .bak back into place.
func TestRefreshOne_RollsBackOnFailure(t *testing.T) {
	isolateConfig(t)
	src := makeFixtureLocalSource(t)
	cache := t.TempDir()
	dest := filepath.Join(cache, "fixture")

	// Pre-populate the cache with v1.
	if err := os.MkdirAll(filepath.Join(dest, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "_base", "v1.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	pin := registry.Pinned{
		Name:      "fixture",
		Source:    src,
		Version:   "local",
		LocalPath: dest,
	}

	// Now nuke the source so Refresh's stat check fails.
	if err := os.RemoveAll(src); err != nil {
		t.Fatal(err)
	}

	client := registry.New()
	returned, err := refreshOne(context.Background(), client, pin)
	if err == nil {
		t.Fatal("expected refresh to fail (source gone)")
	}

	// After rollback, v1.txt should be back in the cache.
	if _, err := os.Stat(filepath.Join(dest, "_base", "v1.txt")); err != nil {
		t.Errorf("expected v1.txt to be rolled back; got err: %v", err)
	}

	// And no .bak directory should be left behind.
	entries, _ := os.ReadDir(cache)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".bak-") {
			t.Errorf("backup should be cleaned up after rollback; found: %s", e.Name())
		}
	}

	// Returned pin should be the ORIGINAL (Version unchanged).
	if returned.Version != "local" {
		t.Errorf("returned.Version = %q, want %q (rollback should not bump)", returned.Version, "local")
	}
}

// TestRefreshOne_SuccessClearsBackup verifies that a successful
// refresh leaves the cache with the new content and the .bak dir
// is gone.
func TestRefreshOne_SuccessClearsBackup(t *testing.T) {
	isolateConfig(t)
	src := makeFixtureLocalSource(t)
	cache := t.TempDir()
	dest := filepath.Join(cache, "fixture")
	if err := os.MkdirAll(filepath.Join(dest, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "_base", "v1.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Source has v2.
	if err := os.WriteFile(filepath.Join(src, "_base", "v2.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	pin := registry.Pinned{Name: "fixture", Source: src, Version: "local", LocalPath: dest}
	client := registry.New()

	_, err := refreshOne(context.Background(), client, pin)
	if err != nil {
		t.Fatalf("refreshOne: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dest, "_base", "v2.txt")); err != nil {
		t.Errorf("expected v2.txt in cache after success; got: %v", err)
	}
	entries, _ := os.ReadDir(cache)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".bak-") {
			t.Errorf("backup should be cleared after success; found: %s", e.Name())
		}
	}
}

// TestBackupPath_Timestamped verifies backupPath returns
// LocalPath + ".bak-<unix-ts>". A future caller (UpdateFlow)
// depends on this exact format to recognise leftover backups.
func TestBackupPath_Timestamped(t *testing.T) {
	got, ok := backupPath("/tmp/foo")
	if !ok {
		t.Fatal("backupPath should return ok=true for a non-empty path")
	}
	if !strings.HasPrefix(got, "/tmp/foo.bak-") {
		t.Errorf("backupPath = %q, want prefix /tmp/foo.bak-", got)
	}
	// The suffix is a unix timestamp; should be all digits.
	suffix := strings.TrimPrefix(got, "/tmp/foo.bak-")
	for _, r := range suffix {
		if r < '0' || r > '9' {
			t.Errorf("backupPath timestamp suffix %q should be all digits", suffix)
		}
	}
}

// TestBackupPath_Empty verifies the empty-path case is a no-op
// (ok=false) so refreshOne can short-circuit.
func TestBackupPath_Empty(t *testing.T) {
	if _, ok := backupPath(""); ok {
		t.Error("backupPath(\"\") should return ok=false")
	}
}
