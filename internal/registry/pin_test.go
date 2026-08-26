package registry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func newPinTestClient(t *testing.T) *Client {
	t.Helper()
	return &Client{CacheDir: t.TempDir()}
}

// TestAdd_SetsPinnedAt verifies that Add() stamps PinnedAt
// on the returned Pinned record, for both local and git specs.
func TestAdd_SetsPinnedAt_Local(t *testing.T) {
	client := newPinTestClient(t)
	ctx := context.Background()

	// Create a minimal valid template dir with spin.toml + _base/.
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "spin.toml"), []byte("name = \"test\""), 0o644); err != nil {
		t.Fatal(err)
	}

	pinned, err := client.Add(ctx, src)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if pinned.PinnedAt == "" {
		t.Error("Add() should set PinnedAt on local sources")
	}
}

// BenchmarkAdd_Local measures Add() performance for local paths.
func BenchmarkAdd_Local(b *testing.B) {
	client := &Client{CacheDir: b.TempDir()}
	ctx := context.Background()
	src := b.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "_base"), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "spin.toml"), []byte("name = \"test\""), 0o644); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		dest := filepath.Join(client.CacheDir, "templates", filepath.Base(src))
		os.RemoveAll(dest)
		_, _ = client.Add(ctx, src)
	}
}

// TestPin_PersistsAndReplaces verifies Pin writes to pinned.json,
// and a second Pin with the same name replaces the record.
func TestPin_PersistsAndReplaces(t *testing.T) {
	client := newPinTestClient(t)
	ctx := context.Background()

	p1 := Pinned{Name: "tpl", Source: "/tmp/a", Version: "v1", LocalPath: "/tmp/a"}
	if err := client.Pin(ctx, p1); err != nil {
		t.Fatal(err)
	}

	pinned, err := client.ListPinned(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pinned) != 1 || pinned[0].Version != "v1" {
		t.Fatalf("after first pin: got %+v", pinned)
	}

	p2 := Pinned{Name: "tpl", Source: "/tmp/b", Version: "v2", LocalPath: "/tmp/b"}
	if err := client.Pin(ctx, p2); err != nil {
		t.Fatal(err)
	}

	pinned, err = client.ListPinned(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pinned) != 1 || pinned[0].Version != "v2" {
		t.Fatalf("after replacement: got %+v", pinned)
	}
}

// TestUnpin_SoftDeletes marks a pin as removed without deleting
// the cache.
func TestUnpin_SoftDeletes(t *testing.T) {
	client := newPinTestClient(t)
	ctx := context.Background()

	p := Pinned{Name: "tpl", Source: "/tmp/a", LocalPath: "/tmp/a"}
	if err := client.Pin(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := client.Unpin(ctx, "tpl"); err != nil {
		t.Fatal(err)
	}

	listed, err := client.ListPinned(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatal("ListPinned should hide soft-deleted pins")
	}

	all, err := client.ListAllPinned(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || !all[0].Removed {
		t.Fatal("ListAllPinned should show soft-deleted pins with Removed=true")
	}
}

// TestUnpin_NotFound is a no-op for a name that doesn't exist.
func TestUnpin_NotFound(t *testing.T) {
	client := newPinTestClient(t)
	ctx := context.Background()
	if err := client.Unpin(ctx, "nonexistent"); err != nil {
		t.Fatal("Unpin of unknown name should be a no-op, got err:", err)
	}
}

// TestPurge_DeletesCache removes the pin record and its on-disk
// cache directory.
func TestPurge_DeletesCache(t *testing.T) {
	client := newPinTestClient(t)
	ctx := context.Background()

	cacheDir := filepath.Join(client.CacheDir, "templates", "tpl")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	p := Pinned{Name: "tpl", Source: "/tmp/a", LocalPath: cacheDir}
	if err := client.Pin(ctx, p); err != nil {
		t.Fatal(err)
	}

	if err := client.Purge(ctx, "tpl"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("cache dir should be deleted after purge")
	}

	all, err := client.ListAllPinned(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatal("pin record should be removed after purge")
	}
}

// TestPurge_NotFound returns an error for a missing name.
func TestPurge_NotFound(t *testing.T) {
	client := newPinTestClient(t)
	ctx := context.Background()
	err := client.Purge(ctx, "nonexistent")
	if err == nil {
		t.Fatal("Purge of unknown name should error")
	}
}

// TestPurge_KeepsSiblingCache verifies purging the first of two pins
// deletes the right cache, a regression guard for the slice-aliasing
// bug that let Purge delete a sibling's LocalPath.
func TestPurge_KeepsSiblingCache(t *testing.T) {
	client := newPinTestClient(t)
	ctx := context.Background()

	firstPath := filepath.Join(client.CacheDir, "templates", "first")
	secondPath := filepath.Join(client.CacheDir, "templates", "second")
	for _, dir := range []string{firstPath, secondPath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.Pin(ctx, Pinned{Name: "first", Source: "/tmp/a", LocalPath: firstPath}); err != nil {
		t.Fatal(err)
	}
	if err := client.Pin(ctx, Pinned{Name: "second", Source: "/tmp/b", LocalPath: secondPath}); err != nil {
		t.Fatal(err)
	}

	if err := client.Purge(ctx, "first"); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if _, err := os.Stat(firstPath); !os.IsNotExist(err) {
		t.Error("purged pin cache should be gone")
	}
	if _, err := os.Stat(secondPath); err != nil {
		t.Errorf("sibling pin cache must survive: %v", err)
	}
	all, err := client.ListAllPinned(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Name != "second" {
		t.Errorf("sibling record must survive purge; got %+v", all)
	}
}
