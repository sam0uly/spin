package template

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/sam0uly/spin/internal/licenses"
	"github.com/sam0uly/spin/internal/params"
)

// Template is a loaded external template, ready to render.
type Template struct {
	Name        string    // directory name, e.g. "rust-cli"
	Source      string    // local path on disk, post-clone
	Repo        string    // git URL, if any
	Spec        string    // original spec the user typed
	SpinToml    *SpinToml // parsed spin.toml
	BaseDir     string    // _base/ inside Source
	PreHookDir  string    // optional _pre/ inside Source
	PostHookDir string    // optional _post/ inside Source
}

// Detect loads the template rooted at dir. A valid template contains
// spin.toml and _base/.
func Detect(dir string) (*Template, error) {
	if strings.HasPrefix(dir, "~/") {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("template: expand home: %w", err)
		}
		dir = filepath.Join(h, dir[2:])
	}
	stPath := filepath.Join(dir, "spin.toml")
	if _, err := os.Stat(stPath); err != nil {
		return nil, fmt.Errorf("template: spin.toml not found in %s", dir)
	}
	base := filepath.Join(dir, "_base")
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("template: _base/ not found in %s", dir)
	}
	st, err := ParseSpinToml(stPath)
	if err != nil {
		return nil, err
	}
	return &Template{
		Name:        filepath.Base(dir),
		Source:      dir,
		SpinToml:    st,
		BaseDir:     base,
		PreHookDir:  filepath.Join(dir, "_pre"),
		PostHookDir: filepath.Join(dir, "_post"),
	}, nil
}

// Render walks the template's _base/ tree and renders each .tmpl file
// against values. It returns a map of output-relative paths to bytes;
// non-templated files are copied verbatim. Paths matching any glob in
// SpinToml.Exclude are skipped entirely. When SpinToml.Include rules
// exist, a file is emitted only if it matches a rule whose If template
// renders truthy; an empty If always matches.
func (t *Template) Render(values map[string]any) (map[string][]byte, error) {
	out := map[string][]byte{}
	// Shared helpers for every file and include rule in this pass.
	funcs := params.FuncMap()
	err := filepath.Walk(t.BaseDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(t.BaseDir, path)
		rel = filepath.ToSlash(rel)
		candidate := stripTmplExt(rel)
		if isExcluded(candidate, t.SpinToml.Exclude) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		include, skipDir, err := t.shouldInclude(rel, candidate, info.IsDir(), values, funcs)
		if err != nil {
			return err
		}
		if skipDir {
			return filepath.SkipDir
		}
		if !include {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(rel) != ".tmpl" {
			// copy non-templated files verbatim
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[candidate] = b
			return nil
		}
		rendered, err := renderFile(path, values, funcs)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		out[candidate] = rendered
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := t.appendLicense(out, values); err != nil {
		return nil, err
	}
	return out, nil
}

// licenseFileNames are the output names that count as the template
// already shipping a license. A generated LICENSE never overwrites them.
var licenseFileNames = []string{"LICENSE", "LICENSE.txt", "LICENSE.md", "COPYING", "COPYING.txt"}

// appendLicense adds a LICENSE to the rendered output when the template
// opted into built-in licensing via a `type = "license"` param. Unknown,
// empty, or "None" values produce nothing. An existing license-named
// file is never overwritten, and without a copyright_holder value the
// SPDX holder token is left in place rather than guessed.
func (t *Template) appendLicense(out map[string][]byte, values map[string]any) error {
	id, _ := values["license"].(string)
	if !licenses.IsKnown(id) {
		return nil
	}
	if hasLicenseFile(out) {
		return nil
	}
	holder, _ := values["copyright_holder"].(string)
	text, err := licenses.Render(id, holder, time.Now().Year())
	if err != nil {
		return err
	}
	out["LICENSE"] = []byte(text)
	return nil
}

// hasLicenseFile reports whether the output already contains a
// license-named file.
func hasLicenseFile(out map[string][]byte) bool {
	for name := range out {
		lower := strings.ToLower(name)
		for _, want := range licenseFileNames {
			if lower == strings.ToLower(want) {
				return true
			}
		}
	}
	return false
}

// shouldInclude evaluates the [[include]] rules for one path. With no
// rules everything is included. Otherwise the path must match a rule
// whose If renders truthy. skipDir tells the walker it can prune a
// directory whose subtree cannot match either.
func (t *Template) shouldInclude(rel, candidate string, isDir bool, values map[string]any, funcs template.FuncMap) (include bool, skipDir bool, err error) {
	if len(t.SpinToml.Include) == 0 {
		return true, false, nil
	}
	matched := false
	for _, rule := range t.SpinToml.Include {
		ok, merr := matchIncludeRule(rule, rel, candidate)
		if merr != nil {
			return false, false, merr
		}
		if !ok {
			continue
		}
		matched = true
		if rule.If == "" {
			return true, false, nil
		}
		truthy, terr := renderBool(rule.If, values, funcs)
		if terr != nil {
			return false, false, terr
		}
		if truthy {
			return true, false, nil
		}
	}
	if !matched {
		return true, false, nil
	}
	if isDir {
		return false, true, nil
	}
	return false, false, nil
}

// matchIncludeRule reports whether the rule's path glob matches rel
// (with .tmpl extension) or candidate (without it).
func matchIncludeRule(rule IncludeRule, rel, candidate string) (bool, error) {
	for _, p := range []string{candidate, rel} {
		if p == "" {
			continue
		}
		if ok, err := matchGlob(rule.Path, p); err != nil {
			return false, fmt.Errorf("include rule %q: invalid glob: %w", rule.Path, err)
		} else if ok {
			return true, nil
		}
	}
	return false, nil
}

// matchGlob reports whether name matches pattern, supporting ** for
// any number of directories on top of filepath.Match semantics.
func matchGlob(pattern, name string) (bool, error) {
	if !strings.Contains(pattern, "**") {
		return filepath.Match(pattern, name)
	}
	parts := strings.Split(pattern, "**")
	if parts[0] == "" {
		rest := strings.TrimPrefix(strings.Join(parts[1:], "**"), "/")
		if rest == "" {
			return true, nil
		}
		return matchAnySuffix(name, rest), nil
	}
	if parts[len(parts)-1] == "" {
		prefix := strings.TrimSuffix(parts[0], "/")
		return strings.HasPrefix(name, prefix), nil
	}
	prefix := parts[0]
	suffix := strings.Join(parts[1:], "**")
	if !strings.HasPrefix(name, prefix) {
		return false, nil
	}
	inner := strings.TrimPrefix(name, prefix)
	inner = strings.TrimPrefix(inner, "/")
	return matchAnySuffix(inner, strings.TrimPrefix(suffix, "/")), nil
}

func matchAnySuffix(name, suffixPattern string) bool {
	if suffixPattern == "" {
		return true
	}
	segments := strings.Split(suffixPattern, "/")
	nameParts := strings.Split(name, "/")
	for i := 0; i <= len(nameParts)-len(segments); i++ {
		candidate := strings.Join(nameParts[i:], "/")
		if ok, _ := filepath.Match(suffixPattern, candidate); ok {
			return true
		}
	}
	return false
}

// renderBool renders a Go template string against values and returns
// whether the result is truthy. Non-bool results follow Go's
// template truthiness rules.
func renderBool(tpl string, values map[string]any, funcs template.FuncMap) (bool, error) {
	t, err := template.New("include").Funcs(funcs).Parse(tpl)
	if err != nil {
		return false, fmt.Errorf("include rule %q: parse: %w", tpl, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, values); err != nil {
		return false, fmt.Errorf("include rule %q: render: %w", tpl, err)
	}
	s := bytes.TrimSpace(buf.Bytes())
	if len(s) == 0 {
		return false, nil
	}
	if s[0] == 't' || s[0] == 'T' || s[0] == '1' || s[0] == 'y' || s[0] == 'Y' {
		return true, nil
	}
	return false, nil
}

func isExcluded(path string, patterns []string) bool {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if ok, err := matchGlob(p, path); err == nil && ok {
			return true
		}
	}
	return false
}

// RenderTo renders the template and writes the files to dest.
func (t *Template) RenderTo(ctx context.Context, dest string, values map[string]any) error {
	files, err := t.Render(values)
	if err != nil {
		return err
	}
	return writeFiles(ctx, dest, files)
}

// RenderToWithPost runs the full pipeline: copy hook assets, run the
// pre-hook, render and write files, then run the post-hook. Finally
// it deletes every spin.toml under dest so the manifest never ships
// in the generated project. The post-hook failing does not skip the
// spin.toml cleanup.
func (t *Template) RenderToWithPost(ctx context.Context, dest string, values map[string]any, opts HookOptions) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", dest, err)
	}
	if err := t.copyPreDir(ctx, dest); err != nil {
		return err
	}
	if err := RunPreHook(ctx, t, values, dest, opts); err != nil {
		return err
	}
	files, err := t.Render(values)
	if err != nil {
		return err
	}
	if err := writeFiles(ctx, dest, files); err != nil {
		return err
	}
	if err := t.copyPostDir(ctx, dest); err != nil {
		return err
	}
	// Best-effort: clean up spin.toml even when the post-hook failed.
	hookErr := RunPostHook(ctx, t, values, dest, opts)
	deleteErr := deleteSpinToml(ctx, dest)
	if hookErr != nil {
		return hookErr
	}
	return deleteErr
}

// deleteSpinToml removes every file named spin.toml under dest. This
// is a defensive walk: templates should not ship the manifest in
// _base/, but if one does it must not reach the user's project.
func deleteSpinToml(ctx context.Context, dest string) error {
	return filepath.Walk(dest, func(path string, info os.FileInfo, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) == "spin.toml" {
			return os.Remove(path)
		}
		return nil
	})
}

// copyPreDir copies the template's optional _pre/ assets into dest so
// pre-hooks can reference them.
func (t *Template) copyPreDir(ctx context.Context, dest string) error {
	return copyHookAssets(ctx, t.PreHookDir, filepath.Join(dest, "_pre"))
}

// copyPostDir copies the template's optional _post/ assets into dest
// so post-hooks can reference them.
func (t *Template) copyPostDir(ctx context.Context, dest string) error {
	return copyHookAssets(ctx, t.PostHookDir, filepath.Join(dest, "_post"))
}

// copyHookAssets copies a hook-asset directory verbatim, without
// rendering. A missing source is a no-op. Targets are checked against
// destRoot to reject path traversal.
func copyHookAssets(ctx context.Context, src, destRoot string) error {
	if src == "" {
		return nil
	}
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	cleanDestRoot := filepath.Clean(destRoot) + string(filepath.Separator)
	return filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destRoot, rel)
		cleanTarget := filepath.Clean(target)
		if !strings.HasPrefix(cleanTarget+string(filepath.Separator), cleanDestRoot) {
			return fmt.Errorf("hook asset path traversal: %q resolves outside %q", rel, destRoot)
		}
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func stripTmplExt(p string) string {
	if len(p) > 5 && p[len(p)-5:] == ".tmpl" {
		return p[:len(p)-5]
	}
	return p
}
