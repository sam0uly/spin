package template

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sam0uly/spin/internal/params"
)

// TestTemplate_RenderToWithPost_DeletesSpinToml verifies that RenderToWithPost removes spin.
func TestTemplate_RenderToWithPost_DeletesSpinToml(t *testing.T) {
	base := t.TempDir()
	dest := t.TempDir()
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a fake spin.toml into the destination (as if a
	// template's _base/ had included it).
	if err := os.WriteFile(filepath.Join(dest, "spin.toml"), []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}

	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
			Post:   nil, // empty -> no post-hook
		},
	}
	if err := tpl.RenderToWithPost(context.Background(), dest, map[string]any{}, HookOptions{}); err != nil {
		t.Fatalf("RenderToWithPost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "spin.toml")); !os.IsNotExist(err) {
		t.Fatalf("spin.toml should be removed from output, stat err=%v", err)
	}
}

// TestTemplate_RenderToWithPost_NestedSpinToml verifies the
// defensive walk also removes spin.toml files in subdirectories.
func TestTemplate_RenderToWithPost_NestedSpinToml(t *testing.T) {
	base := t.TempDir()
	dest := t.TempDir()
	sub := filepath.Join(dest, "sub")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "spin.toml"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
		},
	}
	if err := tpl.RenderToWithPost(context.Background(), dest, map[string]any{}, HookOptions{}); err != nil {
		t.Fatalf("RenderToWithPost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sub, "spin.toml")); !os.IsNotExist(err) {
		t.Fatalf("nested spin.toml should be removed, stat err=%v", err)
	}
}

// TestTemplate_RenderToWithPost_CopiesPreDir verifies that an optional _pre/ directory next to spin.
func TestTemplate_RenderToWithPost_CopiesPreDir(t *testing.T) {
	base := t.TempDir()
	pre := t.TempDir()
	dest := t.TempDir()
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pre, "init.sh"), []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	tpl := &Template{
		BaseDir:    base,
		PreHookDir: pre,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
			Pre:    []PreStep{{Run: "test -f _pre/init.sh"}},
		},
	}
	if err := tpl.RenderToWithPost(context.Background(), dest, map[string]any{}, HookOptions{}); err != nil {
		t.Fatalf("RenderToWithPost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "_pre", "init.sh")); err != nil {
		t.Fatalf("_pre/init.sh should be copied to dest: %v", err)
	}
}

// TestTemplate_RenderToWithPost_CopiesPostDir verifies that an optional _post/ directory next to spin.
func TestTemplate_RenderToWithPost_CopiesPostDir(t *testing.T) {
	base := t.TempDir()
	post := t.TempDir()
	dest := t.TempDir()
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(post, "setup.sh"), []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	tpl := &Template{
		BaseDir:     base,
		PostHookDir: post,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
			Post:   []PostStep{{Run: "test -f _post/setup.sh"}},
		},
	}
	if err := tpl.RenderToWithPost(context.Background(), dest, map[string]any{}, HookOptions{}); err != nil {
		t.Fatalf("RenderToWithPost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "_post", "setup.sh")); err != nil {
		t.Fatalf("_post/setup.sh should be copied to dest: %v", err)
	}
}

// TestTemplate_RenderToWithPost_AutoHookScripts verifies that every file
// in _pre/ and _post/ is executed automatically, in alphabetical order.
func TestTemplate_RenderToWithPost_AutoHookScripts(t *testing.T) {
	base := t.TempDir()
	pre := t.TempDir()
	post := t.TempDir()
	dest := t.TempDir()
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}

	// Multiple scripts in _pre/ run in alphabetical order. The second
	// script checks that the first one already ran.
	if err := os.WriteFile(filepath.Join(pre, "01-first.sh"), []byte("#!/bin/sh\ntouch pre-first\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pre, "02-second.sh"), []byte("#!/bin/sh\nif [ ! -f pre-first ]; then echo 'first did not run'; exit 1; fi\ntouch pre-second\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A non-executable script is run with sh.
	if err := os.WriteFile(filepath.Join(post, "01-setup.sh"), []byte("touch post-setup\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(post, "02-cleanup.sh"), []byte("#!/bin/sh\ntouch post-cleanup\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	tpl := &Template{
		BaseDir:     base,
		PreHookDir:  pre,
		PostHookDir: post,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
		},
	}
	if err := tpl.RenderToWithPost(context.Background(), dest, map[string]any{}, HookOptions{}); err != nil {
		t.Fatalf("RenderToWithPost: %v", err)
	}
	for _, marker := range []string{"pre-first", "pre-second", "post-setup", "post-cleanup"} {
		if _, err := os.Stat(filepath.Join(dest, marker)); err != nil {
			t.Fatalf("marker %q missing: %v", marker, err)
		}
	}
}

// TestUnwrapValue verifies the primitive-extraction helper used by the template renderer and the post-hook.
func TestUnwrapValue(t *testing.T) {
	cases := []struct {
		name string
		in   params.Value
		want any
	}{
		{"string", params.Value{String: "hello"}, "hello"},
		{"int", params.Value{Int: 42}, 42},
		{"bool", params.Value{Bool: true}, true},
		{"list", params.Value{List: []string{"a", "b"}}, []string{"a", "b"}},
		{"empty list", params.Value{List: []string{}}, []string{}},
		{"empty", params.Value{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := UnwrapValue(c.in)
			// Use a type-aware comparison so []string matches
			// by content, not by identity (slices aren't
			// comparable with ==).
			if !equalAny(got, c.want) {
				t.Errorf("UnwrapValue(%+v) = %v (%T), want %v (%T)", c.in, got, got, c.want, c.want)
			}
		})
	}
}

func equalAny(a, b any) bool {
	switch ax := a.(type) {
	case string:
		bx, ok := b.(string)
		return ok && ax == bx
	case int:
		bx, ok := b.(int)
		return ok && ax == bx
	case bool:
		bx, ok := b.(bool)
		return ok && ax == bx
	case []string:
		bx, ok := b.([]string)
		if !ok || len(ax) != len(bx) {
			return false
		}
		for i := range ax {
			if ax[i] != bx[i] {
				return false
			}
		}
		return true
	case nil:
		return b == nil
	}
	return a == b
}

// TestDefaultCacheDir_PrefersXDG verifies the loader's default
// cache directory is the XDG config dir (not the legacy .cache).
func TestDefaultCacheDir_PrefersXDG(t *testing.T) {
	got := defaultCacheDir()
	if !filepath.IsAbs(got) {
		t.Fatalf("defaultCacheDir should be absolute, got %q", got)
	}
	// Should contain "spin/templates" suffix.
	if filepath.Base(got) != "templates" {
		t.Errorf("defaultCacheDir suffix: got %q, want basename=templates", got)
	}
}

// TestRender_Exclude verifies that paths matching any glob in SpinToml.
func TestRender_Exclude(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	// _base/ contains: keep.md (should land), drop.md (exact match),
	// docs/intro.md (glob match), docs/inner.go.tmpl (not excluded,
	// should render and land as docs/inner.go).
	for _, rel := range []string{"keep.md", "drop.md", "docs/intro.md", "docs/inner.go.tmpl"} {
		full := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		body := []byte("hello from " + rel)
		if filepath.Ext(rel) == ".tmpl" {
			body = []byte("package p\n// name={{.name}}\n")
		}
		if err := os.WriteFile(full, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
			Exclude: []string{
				"drop.md",   // exact match
				"docs/*.md", // glob
			},
		},
	}
	out, err := tpl.Render(map[string]any{"name": "myapp"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, ok := out["keep.md"]; !ok {
		t.Errorf("expected keep.md in output; got keys: %v", keysOf(out))
	}
	if _, ok := out["drop.md"]; ok {
		t.Errorf("drop.md should be excluded")
	}
	if _, ok := out["docs/intro.md"]; ok {
		t.Errorf("docs/intro.md should be excluded by glob")
	}
	if _, ok := out["docs/inner.go"]; !ok {
		t.Errorf("docs/inner.go.tmpl should render and land; got keys: %v", keysOf(out))
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestRender_Include verifies that [[include]] rules gate files and
// directories on resolved param values.
func TestRender_Include(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"always.md", "ci/workflows.yml", "auth/middleware.go.tmpl", "grpc/server.go"} {
		full := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		body := []byte("hello from " + rel)
		if filepath.Ext(rel) == ".tmpl" {
			body = []byte("// {{.name}}")
		}
		if err := os.WriteFile(full, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
			Include: []IncludeRule{
				{Path: "always.md"},
				{Path: "ci/**", If: "{{ .ci }}"},
				{Path: "auth/**", If: "{{ has .features \"auth\" }}"},
				{Path: "grpc/**", If: "{{ has .features \"grpc\" }}"},
			},
		},
	}
	out, err := tpl.Render(map[string]any{"name": "myapp", "ci": true, "features": []string{"auth"}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, ok := out["always.md"]; !ok {
		t.Errorf("expected always.md; got keys: %v", keysOf(out))
	}
	if _, ok := out["ci/workflows.yml"]; !ok {
		t.Errorf("expected ci/workflows.yml; got keys: %v", keysOf(out))
	}
	if _, ok := out["auth/middleware.go"]; !ok {
		t.Errorf("expected auth/middleware.go; got keys: %v", keysOf(out))
	}
	if _, ok := out["grpc/server.go"]; ok {
		t.Errorf("grpc/server.go should be excluded when features lacks grpc")
	}
}

// TestRender_IncludeSkipsDirectory verifies that a false [[include]]
// rule for a directory prunes the entire subtree.
func TestRender_IncludeSkipsDirectory(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"keep.md", "optional/a.md", "optional/b.md"} {
		full := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
			Include: []IncludeRule{
				{Path: "keep.md"},
				{Path: "optional/**", If: "{{ .with_optional }}"},
			},
		},
	}
	out, err := tpl.Render(map[string]any{"with_optional": false})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, ok := out["keep.md"]; !ok {
		t.Errorf("expected keep.md; got keys: %v", keysOf(out))
	}
	if _, ok := out["optional/a.md"]; ok {
		t.Errorf("optional/a.md should be excluded")
	}
	if _, ok := out["optional/b.md"]; ok {
		t.Errorf("optional/b.md should be excluded")
	}
}

// TestRenderToWithPost_PreHook verifies that pre-hooks run before
// files are rendered and can write into the destination.
func TestRenderToWithPost_PreHook(t *testing.T) {
	base := t.TempDir()
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "hello.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
			Pre: []PreStep{
				{Run: "touch pre-hook-marker"},
			},
		},
	}
	opts := HookOptions{PrintCommands: true}
	if err := tpl.RenderToWithPost(context.Background(), dest, map[string]any{}, opts); err != nil {
		t.Fatalf("RenderToWithPost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "pre-hook-marker")); err != nil {
		t.Errorf("pre-hook marker missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "hello.md")); err != nil {
		t.Errorf("rendered file missing: %v", err)
	}
}

// TestRenderToWithPost_PreHookFailureStopsRender verifies that a
// failing pre-hook prevents file rendering.
func TestRenderToWithPost_PreHookFailureStopsRender(t *testing.T) {
	base := t.TempDir()
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "hello.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
			Pre: []PreStep{
				{Run: "exit 1"},
			},
		},
	}
	opts := HookOptions{PrintCommands: true}
	err := tpl.RenderToWithPost(context.Background(), dest, map[string]any{}, opts)
	if err == nil {
		t.Fatal("expected pre-hook failure")
	}
	if _, err := os.Stat(filepath.Join(dest, "hello.md")); !os.IsNotExist(err) {
		t.Errorf("hello.md should not be rendered after pre-hook failure")
	}
}

// TestRenderToWithPost_NoHooks skips pre and post hooks.
func TestRenderToWithPost_NoHooks(t *testing.T) {
	base := t.TempDir()
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "hello.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
			Pre: []PreStep{
				{Run: "touch pre-hook-marker"},
			},
			Post: []PostStep{
				{Run: "touch post-hook-marker"},
			},
		},
	}
	opts := HookOptions{NoHooks: true, PrintCommands: true}
	if err := tpl.RenderToWithPost(context.Background(), dest, map[string]any{}, opts); err != nil {
		t.Fatalf("RenderToWithPost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "pre-hook-marker")); !os.IsNotExist(err) {
		t.Errorf("pre-hook marker should not exist with --no-hooks")
	}
	if _, err := os.Stat(filepath.Join(dest, "post-hook-marker")); !os.IsNotExist(err) {
		t.Errorf("post-hook marker should not exist with --no-hooks")
	}
	if _, err := os.Stat(filepath.Join(dest, "hello.md")); err != nil {
		t.Errorf("rendered file should exist: %v", err)
	}
}

// TestRender_Exclude_DoubleStar verifies that `**` in exclude
// patterns matches across multiple directory segments.
func TestRender_Exclude_DoubleStar(t *testing.T) {
	base := t.TempDir()
	for _, rel := range []string{
		"keep.md",
		"a/b/keep.md",
		"a/b/skip.md",
		"a/b/c/skip.md",
	} {
		full := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params:  map[string]params.Spec{},
			Exclude: []string{"**/skip.md"},
		},
	}
	out, err := tpl.Render(map[string]any{"name": "x"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, ok := out["keep.md"]; !ok {
		t.Errorf("keep.md should be included")
	}
	if _, ok := out["a/b/keep.md"]; !ok {
		t.Errorf("a/b/keep.md should be included")
	}
	if _, ok := out["a/b/skip.md"]; ok {
		t.Errorf("a/b/skip.md should be excluded by **")
	}
	if _, ok := out["a/b/c/skip.md"]; ok {
		t.Errorf("a/b/c/skip.md should be excluded by **")
	}
}

// BenchmarkRender_Exclude measures Render with exclude patterns.
func BenchmarkRender_Exclude(b *testing.B) {
	base := b.TempDir()
	for _, rel := range []string{"keep.md", "drop.md", "a/drop.md"} {
		full := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(rel), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params:  map[string]params.Spec{},
			Exclude: []string{"drop.md", "a/*.md"},
		},
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = tpl.Render(map[string]any{"name": "x"})
	}
}

// TestRender_Exclude_DoubleStar_Middle verifies `**` in the
// middle of an exclude pattern matches intermediate directories.
func TestRender_Exclude_DoubleStar_Middle(t *testing.T) {
	base := t.TempDir()
	for _, rel := range []string{"docs/readme.md", "docs/a/readme.md", "docs/a/b/readme.md", "docs/keep.go"} {
		full := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params:  map[string]params.Spec{},
			Exclude: []string{"docs/**/*.md"},
		},
	}
	out, err := tpl.Render(map[string]any{"name": "x"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, ok := out["docs/readme.md"]; ok {
		t.Errorf("docs/readme.md should be excluded")
	}
	if _, ok := out["docs/a/readme.md"]; ok {
		t.Errorf("docs/a/readme.md should be excluded")
	}
	if _, ok := out["docs/a/b/readme.md"]; ok {
		t.Errorf("docs/a/b/readme.md should be excluded")
	}
	if _, ok := out["docs/keep.go"]; !ok {
		t.Errorf("docs/keep.go should be included (not .md)")
	}
}

// TestRender_Exclude_DoubleStar_Trailing verifies a trailing
// `**` excludes everything under a directory.
func TestRender_Exclude_DoubleStar_Trailing(t *testing.T) {
	base := t.TempDir()
	for _, rel := range []string{"internal/x.go", "internal/y.go", "internal/sub/z.go", "keep.go"} {
		full := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params:  map[string]params.Spec{},
			Exclude: []string{"internal/**"},
		},
	}
	out, err := tpl.Render(map[string]any{"name": "x"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, ok := out["internal/x.go"]; ok {
		t.Errorf("internal/x.go should be excluded")
	}
	if _, ok := out["internal/y.go"]; ok {
		t.Errorf("internal/y.go should be excluded")
	}
	if _, ok := out["internal/sub/z.go"]; ok {
		t.Errorf("internal/sub/z.go should be excluded")
	}
	if _, ok := out["keep.go"]; !ok {
		t.Errorf("keep.go should be included")
	}
}

// TestShouldInclude_IsDir verifies the isDir parameter is used correctly: directories that match a rule but have.
func TestShouldInclude_IsDir(t *testing.T) {
	base := t.TempDir()
	subDir := filepath.Join(base, "opt")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "f.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{
				"ci": {Type: params.TypeBool, Default: false},
			},
			Include: []IncludeRule{
				{Path: "opt/**", If: "{{ .ci }}"},
			},
		},
	}
	funcs := params.FuncMap()

	// A file matching the rule with ci=false → not included.
	inc, skip, err := tpl.shouldInclude("opt/f.go", "opt/f.go", false, map[string]any{"ci": false}, funcs)
	if err != nil {
		t.Fatal(err)
	}
	if inc || skip {
		t.Errorf("opt/f.go with ci=false: include=%v skipDir=%v, want false, false", inc, skip)
	}

	// Same file with ci=true → included.
	inc, skip, err = tpl.shouldInclude("opt/f.go", "opt/f.go", false, map[string]any{"ci": true}, funcs)
	if err != nil {
		t.Fatal(err)
	}
	if !inc || skip {
		t.Errorf("opt/f.go with ci=true: include=%v skipDir=%v, want true, false", inc, skip)
	}

	// Directory matching the rule with ci=false → not included, skipDir.
	inc, skip, err = tpl.shouldInclude("opt", "opt", true, map[string]any{"ci": false}, funcs)
	if err != nil {
		t.Fatal(err)
	}
	if inc {
		t.Error("opt dir with ci=false should not be included")
	}
	if !skip {
		t.Error("opt dir with ci=false should return skipDir=true")
	}

	// Same directory with ci=true → included, no skip.
	inc, skip, err = tpl.shouldInclude("opt", "opt", true, map[string]any{"ci": true}, funcs)
	if err != nil {
		t.Fatal(err)
	}
	if !inc || skip {
		t.Errorf("opt dir with ci=true: include=%v skipDir=%v, want true, false", inc, skip)
	}
}

// TestRenderToWithPost_CopiesPreDir verifies that _pre/ assets
// are copied into dest before pre-hooks run.
func TestRenderToWithPost_CopiesPreDir(t *testing.T) {
	base := t.TempDir()
	dest := t.TempDir()
	preDir := filepath.Join(base, "_pre")
	if err := os.MkdirAll(preDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(preDir, "init.sh"), []byte("echo ok"), 0o755); err != nil {
		t.Fatal(err)
	}

	tpl := &Template{
		BaseDir:    base,
		PreHookDir: preDir,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
		},
	}
	opts := HookOptions{NoHooks: true, PrintCommands: true}
	if err := tpl.RenderToWithPost(context.Background(), dest, map[string]any{}, opts); err != nil {
		t.Fatalf("RenderToWithPost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "_pre", "init.sh")); err != nil {
		t.Error("_pre/init.sh should be copied into dest")
	}
}

// TestRenderToWithPost_CopiesPostDir verifies that _post/ assets
// are copied into dest before post-hooks run.
func TestRenderToWithPost_CopiesPostDir(t *testing.T) {
	base := t.TempDir()
	dest := t.TempDir()
	postDir := filepath.Join(base, "_post")
	if err := os.MkdirAll(postDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postDir, "setup.sh"), []byte("echo ok"), 0o755); err != nil {
		t.Fatal(err)
	}

	tpl := &Template{
		BaseDir:     base,
		PostHookDir: postDir,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
		},
	}
	opts := HookOptions{NoHooks: true, PrintCommands: true}
	if err := tpl.RenderToWithPost(context.Background(), dest, map[string]any{}, opts); err != nil {
		t.Fatalf("RenderToWithPost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "_post", "setup.sh")); err != nil {
		t.Error("_post/setup.sh should be copied into dest")
	}
}

// TestRenderToWithPost_ContextCancelled checks that a cancelled
// context stops the pipeline early.
func TestRenderToWithPost_ContextCancelled(t *testing.T) {
	base := t.TempDir()
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "f.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	tpl := &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := tpl.RenderToWithPost(ctx, dest, map[string]any{}, HookOptions{NoHooks: true})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
