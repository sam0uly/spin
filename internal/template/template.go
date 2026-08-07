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
	Name        string    // dir name, e.g. "rust-cli"
	Source      string    // local path on disk (post-clone)
	Repo        string    // git URL, if any
	Spec        string    // original spec the user typed (may differ from Repo/Source when resolved via a registry shorthand)
	SpinToml    *SpinToml // parsed spin.toml
	BaseDir     string    // _base/ inside Source
	PreHookDir  string    // _pre/ inside Source (optional)
	PostHookDir string    // _post/ inside Source (optional)
}

// A valid template has spin.toml and _base/.
func Detect(dir string) (*Template, error) {
	// Expand ~ to home so registry sources and CLI args work uniformly.
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

// Render walks the template's _base/ tree, rendering each .tmpl file
// against the supplied values. The output is a rel-path → bytes map.
//
// values is the resolved param + flag map. Keys ending in `.Name` are
// also available as `.Name` for backwards compat with the existing
// scaffold package.
//
// Files whose path (relative to _base/, with the .tmpl extension
// stripped) matches any glob in t.SpinToml.Exclude are skipped  -
// they never reach the output tree. This is how templates opt out
// of files (e.g. a CI badge, a contributor list) that should stay
// out of the generated project.
//
// If t.SpinToml.Include rules exist, only files matching at least one
// true rule are included. A rule with an empty If always includes.
func (t *Template) Render(values map[string]any) (map[string][]byte, error) {
	out := map[string][]byte{}
	// Build the template helpers once and reuse them for every file
	// and every [[include]] rule in this pass.
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

// licenseFileNames are the output names that count as "the template
// already ships a license file". A generated LICENSE never overwrites
// any of them.
var licenseFileNames = []string{"LICENSE", "LICENSE.txt", "LICENSE.md", "COPYING", "COPYING.txt"}

// appendLicense adds a LICENSE file to the rendered output when the
// template opted into built-in licensing. The resolved `license` value
// (from a `type = "license"` param) must be a known built-in license;
// empty, "None", "Proprietary", and unknown values never produce a
// file. An existing license-named file in the output is never
// overwritten. The copyright holder comes from the optional
// `copyright_holder` value; when absent, the SPDX token is left in
// place so the file never claims an ownership it was not given. The
// year is always the current year.
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
// license-named file (case-insensitive).
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

// shouldInclude evaluates the [[include]] rules for a path. If no
// rules exist, the file is included. Otherwise the path must match
// at least one rule whose If template renders truthy. Directories
// with no matching true rule return skipDir=true so the walk can
// prune the subtree.
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

// matchGlob reports whether name matches pattern. It supports ** to
// match any number of directories, plus single-segment * like
// filepath.Match. Patterns without ** fall back to filepath.Match.
func matchGlob(pattern, name string) (bool, error) {
	if !strings.Contains(pattern, "**") {
		return filepath.Match(pattern, name)
	}
	parts := strings.Split(pattern, "**")
	// pattern starts with **
	if parts[0] == "" {
		rest := strings.TrimPrefix(strings.Join(parts[1:], "**"), "/")
		if rest == "" {
			return true, nil
		}
		return matchAnySuffix(name, rest), nil
	}
	// pattern ends with **
	if parts[len(parts)-1] == "" {
		prefix := strings.TrimSuffix(parts[0], "/")
		return strings.HasPrefix(name, prefix), nil
	}
	// prefix/**/suffix
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

// RenderTo writes the rendered files to dest. Same path-traversal
// guard as scaffold.emit.
func (t *Template) RenderTo(ctx context.Context, dest string, values map[string]any) error {
	files, err := t.Render(values)
	if err != nil {
		return err
	}
	return writeFiles(ctx, dest, files)
}

// RenderToWithPost is the full v2.0 template pipeline:
//  1. Render the template to an in-memory file map.
//  2. Write the files to dest (path-traversal-safe via writeFiles).
//  3. Run the post-hook (if any) in dest.
//  4. Walk dest and delete every spin.toml file found (TPL-16:
//     "spin.toml is deleted from the output after a successful
//     render").
//
// Returns the first non-nil error encountered. The post-hook and
// the spin.toml deletion are best-effort cleanup operations: if
// the post-hook fails, the spin.toml deletion still runs.
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
	// Post-hook: best-effort. Even if it fails, we still attempt
	// to delete spin.toml from the output (TPL-16).
	hookErr := RunPostHook(ctx, t, values, dest, opts)
	deleteErr := deleteSpinToml(ctx, dest)
	if hookErr != nil {
		return hookErr
	}
	return deleteErr
}

// deleteSpinToml walks dest and removes any file named spin.toml.
// TPL-16 specifies that the manifest must not end up in the user's
// project; a defensive walk handles the case where a template
// accidentally included a spin.toml in _base/ (instead of relying
// on the manifest never being rendered in the first place).
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

// copyPreDir copies the template's optional _pre/ directory into
// dest/_pre/ so pre-hooks can reference scripts or assets before the
// main _base/ files are written.
func (t *Template) copyPreDir(ctx context.Context, dest string) error {
	return copyHookAssets(ctx, t.PreHookDir, filepath.Join(dest, "_pre"))
}

// copyPostDir copies the template's optional _post/ directory into
// dest/_post/ so post-hooks can reference scripts or assets stored
// alongside the template.
func (t *Template) copyPostDir(ctx context.Context, dest string) error {
	return copyHookAssets(ctx, t.PostHookDir, filepath.Join(dest, "_post"))
}

// copyHookAssets copies a hook-asset directory verbatim (no .tmpl
// rendering). The destination must stay inside destRoot. Missing src
// is a no-op; any other error is returned.
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
