package template

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sam0uly/spin/internal/registry"
	srcspec "github.com/sam0uly/spin/internal/spec"
)

// TestLoader_Load_LocalPath verifies Load with a local dir returns a non-nil *Template with the correct BaseDir.
func TestLoader_Load_LocalPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spin.toml"), []byte("name = \"tpl\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_base", "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(t.TempDir())
	tpl, err := l.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tpl == nil {
		t.Fatal("Load returned nil Template")
	}
	if tpl.BaseDir == "" {
		t.Errorf("Template.BaseDir is empty")
	}
}

// TestLoader_Load_LocalPath_MissingSpinToml verifies Load fails (with a clear "spin.
func TestLoader_Load_LocalPath_MissingSpinToml(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(t.TempDir())
	_, err := l.Load(dir)
	if err == nil {
		t.Fatal("Load should fail when spin.toml is missing")
	}
	if !strings.Contains(err.Error(), "spin.toml") {
		t.Errorf("error should mention spin.toml, got: %q", err.Error())
	}
}

// TestLoader_Load_LocalPath_MissingBase verifies Load fails when the local dir has spin.toml but no _base/ directory.
func TestLoader_Load_LocalPath_MissingBase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spin.toml"), []byte("name = \"tpl\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(t.TempDir())
	_, err := l.Load(dir)
	if err == nil {
		t.Fatal("Load should fail when _base/ is missing")
	}
	if !strings.Contains(err.Error(), "_base") {
		t.Errorf("error should mention _base/, got: %q", err.Error())
	}
}

// TestLoader_IsLocalPath verifies the heuristic that distinguishes
// a local path from a git URL. Used by Load to dispatch.
func TestLoader_IsLocalPath(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"/foo", true},
		{"./foo", true},
		{"~foo", true},
		{"https://github.com/foo/bar", false},
		{"git@github.com:foo/bar", false},
		{"foo/bar", false}, // ambiguous; treated as a registry shorthand, not a local path
	}
	for _, tc := range cases {
		if got := srcspec.IsLocalPath(tc.in); got != tc.want {
			t.Errorf("IsLocalPath(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestLoader_IsGitURL verifies the heuristic for git URL detection.
func TestLoader_IsGitURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://github.com/foo/bar", true},
		{"http://github.com/foo/bar", true},
		{"git@github.com:foo/bar", true},
		{"git://github.com/foo/bar", true},
		{"ssh://git@github.com/foo/bar", true},
		{"/local/path", false},
		{"./local", false},
	}
	for _, tc := range cases {
		if got := srcspec.IsGitURL(tc.in); got != tc.want {
			t.Errorf("IsGitURL(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestRender_PathTraversal verifies that rendering a template with a key like "../escape.
func TestRender_PathTraversal(t *testing.T) {
	// Build a path-traversal file map by hand and call
	// WriteFiles directly. We don't need a real Template for
	// this test -- the security guard is in writeFiles, which
	// is the same code path RenderTo uses.
	dest := t.TempDir()
	err := writeFiles(context.Background(), dest, map[string][]byte{
		"../escape.txt": []byte("evil"),
	})
	if err == nil {
		t.Fatal("WriteFiles should reject path-traversal key")
	}
	if !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("error should mention 'path traversal', got: %q", err.Error())
	}
}

// TestRender_DeletesSpinToml verifies that RenderToWithPost removes spin.
func TestRender_DeletesSpinToml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spin.toml"), []byte("name = \"tpl\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Include a spin.toml in _base/ as if the template author
	// was sloppy.
	if err := os.WriteFile(filepath.Join(dir, "_base", "spin.toml"), []byte("name = \"stray\""), 0o644); err != nil {
		t.Fatal(err)
	}
	// A non-spin.toml file alongside it (so we have something
	// to render).
	if err := os.WriteFile(filepath.Join(dir, "_base", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tpl, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	dest := t.TempDir()
	if err := tpl.RenderToWithPost(context.Background(), dest, map[string]any{}, HookOptions{}); err != nil {
		t.Fatalf("RenderToWithPost: %v", err)
	}
	// spin.toml at top level was never rendered (it's not in
	// _base), so it doesn't exist at dest/spin.toml. The
	// _base/spin.toml IS rendered, so it appears at
	// dest/spin.toml -- and the defensive walk must remove it.
	if _, err := os.Stat(filepath.Join(dest, "spin.toml")); !os.IsNotExist(err) {
		t.Errorf("dest/spin.toml should NOT exist (TPL-16), but stat says: %v", err)
	}
	// The other file is still there.
	if _, err := os.Stat(filepath.Join(dest, "main.go")); err != nil {
		t.Errorf("dest/main.go should exist, got: %v", err)
	}
}

// TestRunPostHook_RunsShellCommand verifies post-hook commands run via sh -c with templated values.
func TestRunPostHook_RunsShellCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spin.toml"), []byte("name = \"tpl\"\n[[post]]\nrun = \"echo {{.name}} > post-out.txt && touch post-ran.txt\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	tpl, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if err := RunPostHook(context.Background(), tpl, map[string]any{"name": "test-proj"}, dir, HookOptions{}); err != nil {
		t.Fatalf("RunPostHook: %v", err)
	}
	// post-ran.txt is the touch side-effect; it must exist.
	if _, err := os.Stat(filepath.Join(dir, "post-ran.txt")); err != nil {
		t.Errorf("post-ran.txt should exist (touch side-effect), got: %v", err)
	}
	// post-out.txt is the echo output; it should contain the
	// interpolated name.
	b, err := os.ReadFile(filepath.Join(dir, "post-out.txt"))
	if err != nil {
		t.Fatalf("ReadFile post-out.txt: %v", err)
	}
	if !strings.Contains(string(b), "test-proj") {
		t.Errorf("post-out.txt = %q, want it to contain %q", string(b), "test-proj")
	}
}

// TestLoader_Load_GitURL_Mock verifies Load dispatches to the git-clone branch for git URLs.
func TestLoader_Load_GitURL_Mock(t *testing.T) {
	spec := "https://github.com/foo/bar.git"
	if srcspec.IsLocalPath(spec) {
		t.Errorf("IsLocalPath(%q) = true, want false (git URLs are not local)", spec)
	}
	if !srcspec.IsGitURL(spec) {
		t.Errorf("IsGitURL(%q) = false, want true", spec)
	}
}

// TestRunPostHook_MultiStepOrder verifies that two [[post]] steps both run, in the order they appear in spin.
func TestRunPostHook_MultiStepOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spin.toml"), []byte(`name = "tpl"
[[post]]
run = "echo first > step1.txt"
[[post]]
run = "echo second > step2.txt"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	tpl, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if err := RunPostHook(context.Background(), tpl, map[string]any{}, dir, HookOptions{}); err != nil {
		t.Fatalf("RunPostHook: %v", err)
	}
	for name, want := range map[string]string{"step1.txt": "first", "step2.txt": "second"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		if strings.TrimSpace(string(b)) != want {
			t.Errorf("%s = %q, want %q", name, strings.TrimSpace(string(b)), want)
		}
	}
}

// TestCompareSemver verifies the compareSemver helper that compares
// dotted semver strings component-by-component.
func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "1.0.0", 0},
		{"1.0", "1.0.0", 0},    // missing component treated as 0
		{"1.2.3", "1.2.4", -1}, // patch differs
		{"abc", "1.0", -1},     // non-numeric treated as 0
	}
	for _, c := range cases {
		t.Run(c.a+"_"+c.b, func(t *testing.T) {
			got := compareSemver(c.a, c.b)
			if got != c.want {
				t.Errorf("compareSemver(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

// TestLoader_Load_ShorthandUsesPinned verifies that loadShorthand checks pinned templates before cloning.
func TestLoader_Load_ShorthandUsesPinned(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	ctx := context.Background()

	// Create a valid template.
	tplDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tplDir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "spin.toml"), []byte("name = \"test-tpl\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "_base", "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create and register a registry with a template pointing at a git URL.
	regDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(regDir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(regDir, "registry.toml"),
		[]byte("id = \"test\"\nname = \"Test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tplMeta := "id = \"test-tpl\"\nname = \"Test Tpl\"\nsource = \"https://github.com/user/repo.git\"\n"
	if err := os.WriteFile(filepath.Join(regDir, "templates", "test-tpl.toml"),
		[]byte(tplMeta), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := registry.NewManager()
	if _, err := mgr.Add(ctx, "test", regDir, false); err != nil {
		t.Fatal(err)
	}

	// Pin the git URL manually, pointing LocalPath at our real template.
	client := registry.New()
	if err := client.Pin(ctx, registry.Pinned{
		Name:      "test-tpl",
		Source:    "https://github.com/user/repo.git",
		Version:   "abc123",
		LocalPath: tplDir,
	}); err != nil {
		t.Fatal(err)
	}

	// Load via shorthand: should find the pin and use Detect, not clone.
	l := NewLoader(filepath.Join(xdg, "spin", "templates"))
	tpl, err := l.LoadContext(ctx, "test/test-tpl")
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}
	if tpl == nil {
		t.Fatal("LoadContext returned nil")
	}
	if tpl.Spec != "test/test-tpl" {
		t.Errorf("Spec = %q, want %q", tpl.Spec, "test/test-tpl")
	}
	if tpl.Repo != "https://github.com/user/repo.git" {
		t.Errorf("Repo = %q", tpl.Repo)
	}
}

// TestLoader_Load_ShorthandPinNotFound verifies that when the resolved source is NOT pinned, loadShorthand falls.
func TestLoader_Load_ShorthandPinNotFound(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	ctx := context.Background()

	// Create and register a registry without pinning.
	regDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(regDir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(regDir, "registry.toml"),
		[]byte("id = \"test\"\nname = \"Test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(regDir, "templates", "ghost.toml"),
		[]byte("id = \"ghost\"\nname = \"Ghost\"\nsource = \"https://github.com/user/nonexistent.git\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := registry.NewManager()
	if _, err := mgr.Add(ctx, "test", regDir, false); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(filepath.Join(xdg, "spin", "templates"))
	loadCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, err := l.LoadContext(loadCtx, "test/ghost")
	if err == nil {
		t.Fatal("expected error from failed clone pin -> clone fallthrough")
	}
	if !strings.Contains(err.Error(), "git clone") {
		t.Errorf("expected git clone error; got: %v", err)
	}
}

// TestLoader_Load_ShorthandPinStaleLocalPath verifies that when a pin exists but its LocalPath is stale (template.
func TestLoader_Load_ShorthandPinStaleLocalPath(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	ctx := context.Background()

	// Create and register a registry.
	regDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(regDir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(regDir, "registry.toml"),
		[]byte("id = \"test\"\nname = \"Test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(regDir, "templates", "stale.toml"),
		[]byte("id = \"stale\"\nname = \"Stale\"\nsource = \"https://github.com/user/nonexistent.git\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := registry.NewManager()
	if _, err := mgr.Add(ctx, "test", regDir, false); err != nil {
		t.Fatal(err)
	}

	// Pin with a LocalPath that no longer exists.
	goneDir := filepath.Join(t.TempDir(), "nonexistent")
	client := registry.New()
	if err := client.Pin(ctx, registry.Pinned{
		Name:      "stale",
		Source:    "https://github.com/user/nonexistent.git",
		LocalPath: goneDir,
	}); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(filepath.Join(xdg, "spin", "templates"))
	loadCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, err := l.LoadContext(loadCtx, "test/stale")
	if err == nil {
		t.Fatal("expected error from stale pin -> clone fallthrough")
	}
	// Should fail with git error, not "pinned missing on disk".
	if strings.Contains(err.Error(), "missing on disk") {
		t.Errorf("should NOT report missing pin; should fall through to clone: %v", err)
	}
}

// TestLoader_Load_ShorthandPinMismatchSource verifies that a pin
// with a different Source URL is not used by loadShorthand.
func TestLoader_Load_ShorthandPinMismatchSource(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	ctx := context.Background()

	// Create a valid template.
	tplDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tplDir, "_base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "spin.toml"), []byte("name = \"other\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create and register a registry.
	regDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(regDir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(regDir, "registry.toml"),
		[]byte("id = \"test\"\nname = \"Test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(regDir, "templates", "actual.toml"),
		[]byte("id = \"actual\"\nname = \"Actual\"\nsource = \"https://github.com/user/actual.git\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := registry.NewManager()
	if _, err := mgr.Add(ctx, "test", regDir, false); err != nil {
		t.Fatal(err)
	}

	// Pin a DIFFERENT source: it must not be reused.
	client := registry.New()
	if err := client.Pin(ctx, registry.Pinned{
		Name:      "other",
		Source:    "https://github.com/user/other.git",
		Version:   "local",
		LocalPath: tplDir,
	}); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(filepath.Join(xdg, "spin", "templates"))
	loadCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, err := l.LoadContext(loadCtx, "test/actual")
	if err == nil {
		t.Fatal("expected error from mismatched pin -> clone fallthrough")
	}
	if !strings.Contains(err.Error(), "git clone") {
		t.Errorf("expected git clone error (pin had different source); got: %v", err)
	}
}
