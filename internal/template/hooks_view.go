package template

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// HookView is one reviewable hook entry for the interactive TUI: an
// inline [[pre]]/[[post]] step (Run set) or a script file from _pre/
// or _post/ (File set, IsFile true).
type HookView struct {
	// Phase is "pre" or "post".
	Phase string
	// Run is the inline shell command for [[pre]]/[[post]] steps.
	Run string
	// File is the absolute path to the script for _pre/_post hooks.
	File string
	// IsFile reports whether this entry refers to a script file.
	IsFile bool
}

// CollectHooks returns every hook the template would run, in execution
// order: inline [[pre]] steps, then _pre/ scripts, then inline [[post]]
// steps, then _post/ scripts. Missing _pre/_post directories are skipped.
func CollectHooks(t *Template) []HookView {
	if t == nil || t.SpinToml == nil {
		return nil
	}
	var out []HookView
	for _, s := range t.SpinToml.Pre {
		if strings.TrimSpace(strings.TrimSpace(s.Run)) == "" {
			continue
		}
		out = append(out, HookView{Phase: "pre", Run: s.Run})
	}
	out = append(out, hookFiles(t.PreHookDir, "pre")...)
	for _, s := range t.SpinToml.Post {
		if strings.TrimSpace(strings.TrimSpace(s.Run)) == "" {
			continue
		}
		out = append(out, HookView{Phase: "post", Run: s.Run})
	}
	out = append(out, hookFiles(t.PostHookDir, "post")...)
	return out
}

// hookFiles lists non-hidden, non-directory files in dir, sorted by name,
// as file-based hook entries for the given phase.
func hookFiles(dir, phase string) []HookView {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)
	out := make([]HookView, 0, len(paths))
	for _, p := range paths {
		out = append(out, HookView{Phase: phase, File: p, IsFile: true})
	}
	return out
}
