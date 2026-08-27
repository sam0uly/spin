package template

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
)

// RunPostHook executes the template's [[post]] steps plus any scripts
// in _post/, in that order, after files are written but before
// spin.toml is removed from the output. Each command is rendered
// against the resolved values and run via sh -c in dir. The first
// failure stops the hook. A missing post section is a no-op.
func RunPostHook(ctx context.Context, t *Template, values map[string]any, dir string, opts HookOptions) error {
	if t == nil || t.SpinToml == nil {
		return nil
	}
	steps := make([]hookStep, 0, len(t.SpinToml.Post))
	for _, s := range t.SpinToml.Post {
		steps = append(steps, hookStep(s))
	}
	scripts, err := autoHookScripts(dir, "_post")
	if err != nil {
		return fmt.Errorf("post-hook: list scripts: %w", err)
	}
	for _, cmd := range scripts {
		steps = append(steps, hookStep{Run: cmd})
	}
	if len(steps) == 0 {
		return nil
	}
	return runHooks(ctx, "post", steps, values, dir, opts)
}

// renderHook renders a hook command as a text/template against the
// resolved values. Deliberately no helper funcs: hooks are thin shell
// wrappers, not full templating passes.
func renderHook(cmd string, values map[string]any) (string, error) {
	t, err := template.New("post").Parse(cmd)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, values); err != nil {
		return "", err
	}
	return buf.String(), nil
}
