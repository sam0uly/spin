package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/sam0uly/spin/internal/registry"
)

// TestPinnedSearchEntries_ExactMatch verifies pinnedSearchEntries
// returns the pinned template when its name matches the query.
func TestPinnedSearchEntries_ExactMatch(t *testing.T) {
	dir := t.TempDir()
	client := &registry.Client{CacheDir: dir}
	ctx := context.Background()

	tplDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tplDir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "spin.toml"), []byte(`name = "my-tpl"
description = "Test description"
tags = ["go", "cli"]
type = "cli"
language = "go"`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := client.Pin(ctx, registry.Pinned{
		Name:      "my-tpl",
		Source:    tplDir,
		LocalPath: tplDir,
	}); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	entries := pinnedSearchEntries(cmd, client, "my-tpl")
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].ID != "my-tpl" {
		t.Errorf("ID = %q", entries[0].ID)
	}
}

// TestPinnedSearchEntries_NoMatch verifies no results are returned
// when the query doesn't match anything.
func TestPinnedSearchEntries_NoMatch(t *testing.T) {
	dir := t.TempDir()
	client := &registry.Client{CacheDir: dir}
	ctx := context.Background()

	tplDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tplDir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "spin.toml"), []byte(`name = "my-tpl"`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := client.Pin(ctx, registry.Pinned{
		Name:      "my-tpl",
		Source:    tplDir,
		LocalPath: tplDir,
	}); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	entries := pinnedSearchEntries(cmd, client, "nonexistent")
	if len(entries) != 0 {
		t.Errorf("got %d entries for nonexistent query, want 0", len(entries))
	}
}

// TestPinnedSearchEntries_EmptyPins verifies no crash or results
// when no pins exist.
func TestPinnedSearchEntries_EmptyPins(t *testing.T) {
	dir := t.TempDir()
	client := &registry.Client{CacheDir: dir}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	entries := pinnedSearchEntries(cmd, client, "anything")
	if len(entries) != 0 {
		t.Errorf("got %d entries with no pins, want 0", len(entries))
	}
}

// TestPinnedSearchEntries_SubstringMatch verifies partial query
// matching works (description, tags, type, language).
func TestPinnedSearchEntries_SubstringMatch(t *testing.T) {
	dir := t.TempDir()
	client := &registry.Client{CacheDir: dir}
	ctx := context.Background()

	tplDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tplDir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "spin.toml"), []byte(`name = "special-tpl"
description = "A Rust TUI application"
tags = ["tui", "rust"]
type = "tui"
language = "rust"`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := client.Pin(ctx, registry.Pinned{
		Name:      "special-tpl",
		Source:    tplDir,
		LocalPath: tplDir,
	}); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	tests := []struct {
		query string
		found bool
	}{
		{"special-tpl", true},  // ID match
		{"Rust", true},         // Description match
		{"rust", true},         // Language match (lowercase query)
		{"tui", true},          // Type + tag match
		{"nonexistent", false}, // No match
		{"xyz", false},         // No match
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			entries := pinnedSearchEntries(cmd, client, tc.query)
			got := len(entries) > 0
			if got != tc.found {
				t.Errorf("query %q: got found=%v, want %v", tc.query, got, tc.found)
			}
		})
	}
}

// TestPinnedSearchEntries_MultiplePins verifies multiple pins are
// returned, filtered by query.
func TestPinnedSearchEntries_MultiplePins(t *testing.T) {
	dir := t.TempDir()
	client := &registry.Client{CacheDir: dir}
	ctx := context.Background()

	for _, name := range []string{"go-cli", "rust-api", "py-tool"} {
		tplDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(tplDir, "_base"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tplDir, "spin.toml"),
			[]byte("name = \""+name+"\"\nlanguage = \"unknown\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := client.Pin(ctx, registry.Pinned{
			Name:      name,
			Source:    tplDir,
			LocalPath: tplDir,
		}); err != nil {
			t.Fatal(err)
		}
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	entries := pinnedSearchEntries(cmd, client, "cli")
	if len(entries) != 1 || entries[0].ID != "go-cli" {
		t.Errorf("expected 1 entry (go-cli), got %d", len(entries))
	}

	// Empty query returns all.
	allEntries := pinnedSearchEntries(cmd, client, "")
	if len(allEntries) != 3 {
		t.Errorf("expected 3 entries for empty query, got %d", len(allEntries))
	}
}

// TestPinnedSearchEntries_MissingLocalPath verifies a pinned template
// without a local path (e.g. partial pin state) doesn't crash.
func TestPinnedSearchEntries_MissingLocalPath(t *testing.T) {
	dir := t.TempDir()
	client := &registry.Client{CacheDir: dir}
	ctx := context.Background()

	if err := client.Pin(ctx, registry.Pinned{
		Name:   "ghost",
		Source: "https://github.com/user/repo.git",
	}); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	entries := pinnedSearchEntries(cmd, client, "ghost")
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Source != "https://github.com/user/repo.git" {
		t.Errorf("unexpected source: %q", entries[0].Source)
	}
}

// TestPinnedSearchEntries_QueryWhitespace verifies query
// trimming (not exact match, just contains).
func TestPinnedSearchEntries_QueryWhitespace(t *testing.T) {
	dir := t.TempDir()
	client := &registry.Client{CacheDir: dir}
	ctx := context.Background()

	tplDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tplDir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "spin.toml"), []byte(`name = "test-app"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := client.Pin(ctx, registry.Pinned{
		Name:      "test-app",
		Source:    tplDir,
		LocalPath: tplDir,
	}); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	// The query isn't trimmed internally; the CLI splits args by whitespace,
	// so a single word "test" is what we expect.
	entries := pinnedSearchEntries(cmd, client, "test")
	if len(entries) != 1 {
		t.Errorf("expected 1 entry for 'test', got %d", len(entries))
	}
}

// TestPinnedSearchEntries_EmptyQuery verifies empty query returns all.
func TestPinnedSearchEntries_EmptyQuery(t *testing.T) {
	dir := t.TempDir()
	client := &registry.Client{CacheDir: dir}
	ctx := context.Background()

	tplDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tplDir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "spin.toml"), []byte(`name = "x"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := client.Pin(ctx, registry.Pinned{
		Name:      "x",
		Source:    tplDir,
		LocalPath: tplDir,
	}); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	entries := pinnedSearchEntries(cmd, client, "")
	if len(entries) != 1 {
		t.Errorf("expected 1 entry for empty query, got %d", len(entries))
	}
}

// TestPinnedSearchEntries_SourceDedup covers pinned-entry metadata; the
// dedup itself happens in runSearch.
func TestPinnedSearchEntries_ReturnsMetadata(t *testing.T) {
	dir := t.TempDir()
	client := &registry.Client{CacheDir: dir}
	ctx := context.Background()

	tplDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tplDir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "spin.toml"), []byte(`name = "data-tpl"
description = "Data driven template"
tags = ["data", "csv"]
type = "lib"
language = "python"`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := client.Pin(ctx, registry.Pinned{
		Name:      "data-tpl",
		Source:    tplDir,
		Version:   "v1.0.0",
		LocalPath: tplDir,
	}); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	entries := pinnedSearchEntries(cmd, client, "data")
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Name != "data-tpl" || e.Description != "Data driven template" || e.Type != "lib" || e.Language != "python" || e.Version != "v1.0.0" {
		t.Errorf("unexpected metadata: %+v", e)
	}
	if len(e.Tags) != 2 || e.Tags[0] != "data" {
		t.Errorf("unexpected tags: %v", e.Tags)
	}
}
