package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sam0uly/spin/internal/template"
)

// keyPress builds a tea.KeyPressMsg for a single key.
func keyPress(s string) tea.KeyPressMsg {
	if s == "enter" {
		return tea.KeyPressMsg{Code: '\r'}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

// writeFixtureTemplate creates a minimal template with inline pre/post steps plus script files.
func writeFixtureTemplate(t *testing.T) *template.Template {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spin.toml"), []byte(`
name = "fixture"
[params]
name = { type = "text", default = "world" }
[[pre]]
run = "echo preparing to scaffold"
[[post]]
run = "echo done post"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "_base")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "hello.txt"), []byte("hi {{.name}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	pre := filepath.Join(dir, "_pre")
	if err := os.MkdirAll(pre, 0o755); err != nil {
		t.Fatal(err)
	}
	// Non-executable so the runner uses `sh _pre/hi.sh`.
	if err := os.WriteFile(filepath.Join(pre, "hi.sh"), []byte("echo from-file"), 0o644); err != nil {
		t.Fatal(err)
	}
	loader := template.NewLoader("")
	tpl, err := loader.LoadContext(context.Background(), dir)
	if err != nil {
		t.Fatalf("load fixture template: %v", err)
	}
	return tpl
}

// pump drains streamed output messages until the run completes.
func pump(t *testing.T, m hooksModel, cmd tea.Cmd) hooksModel {
	t.Helper()
	for cmd != nil {
		msg := cmd()
		switch x := msg.(type) {
		case runLineMsg:
			m, cmd = m.update(x)
		case runDoneMsg:
			m, _ = m.update(x)
			cmd = nil
		default:
			t.Fatalf("unexpected message type %T", msg)
		}
	}
	return m
}

// TestHooksTUI_SelectedHookContent verifies the right pane previews the selected hook.
func TestHooksTUI_SelectedHookContent(t *testing.T) {
	tpl := writeFixtureTemplate(t)
	resolved := map[string]any{"name": "myapp", "project_name": "myapp"}
	m := newHooksModel(tpl, newTUIStyles(), 100, 30, resolved, context.Background(), t.TempDir(), "myapp", false, false, false)

	// Hook order: [inline pre(0), _pre/hi.sh(1), inline post(2)].
	// Index 0 is an inline pre command.
	m.list.Select(0)
	inline := m.selectedHookContent(m.list.Index())
	if !strings.Contains(inline, "echo preparing to scaffold") {
		t.Fatalf("expected inline command in pane, got:\n%s", inline)
	}
	if strings.Contains(inline, "from-file") {
		t.Fatalf("inline pane must not show the file's body, got:\n%s", inline)
	}

	// Index 1 is the _pre/hi.sh script file; the pane shows its body.
	m.list.Select(1)
	file := m.selectedHookContent(m.list.Index())
	if !strings.Contains(file, "from-file") {
		t.Fatalf("expected script file body in pane, got:\n%s", file)
	}
	if !strings.Contains(file, "_pre/hi.sh") {
		t.Fatalf("expected script path in pane, got:\n%s", file)
	}

	// Navigating the list updates the pane (back to inline post).
	m.list.Select(2)
	post := m.selectedHookContent(m.list.Index())
	if !strings.Contains(post, "echo done post") {
		t.Fatalf("expected inline post command in pane, got:\n%s", post)
	}
}

// TestHooksTUI_ModalStateMachine verifies the run-all modal (opened with
// R) toggles and closes without running.
func TestHooksTUI_ModalStateMachine(t *testing.T) {
	tpl := writeFixtureTemplate(t)
	resolved := map[string]any{"name": "myapp", "project_name": "myapp"}
	m := newHooksModel(tpl, newTUIStyles(), 100, 30, resolved, context.Background(), t.TempDir(), "myapp", false, false, false)

	if m.modalOpen {
		t.Fatal("modal should start closed")
	}
	// R opens the run-all modal.
	m, _ = m.update(keyPress("R"))
	if !m.modalOpen {
		t.Fatal("expected modal to open on R")
	}
	if m.modalChoice != 0 {
		t.Fatalf("expected default choice Run (0), got %d", m.modalChoice)
	}
	m, _ = m.update(keyPress("l"))
	if m.modalChoice != 1 {
		t.Fatalf("expected Skip (1) after 'l', got %d", m.modalChoice)
	}
	m, _ = m.update(keyPress("h"))
	if m.modalChoice != 0 {
		t.Fatalf("expected Run (0) after 'h', got %d", m.modalChoice)
	}
	m, _ = m.update(keyPress("esc"))
	if m.modalOpen {
		t.Fatal("expected modal to close on esc")
	}
	if m.didRun {
		t.Fatal("esc must not trigger a run")
	}
}

// TestHooksTUI_RunAllScaffolds verifies that submitting the modal runs
// the full scaffold and sets didRun so runNew skips its own render.
func TestHooksTUI_RunAllScaffolds(t *testing.T) {
	tpl := writeFixtureTemplate(t)
	dest := t.TempDir()
	resolved := map[string]any{"name": "myapp", "project_name": "myapp"}
	m := newHooksModel(tpl, newTUIStyles(), 100, 30, resolved, context.Background(), dest, "myapp", false, false, false)

	// R opens modal, enter submits (Run) -> full scaffold.
	m, _ = m.update(keyPress("R"))
	m, cmd := m.update(keyPress("enter"))
	if !m.didRun {
		t.Fatal("expected didRun after submit")
	}
	if !m.running {
		t.Fatal("expected running true after full scaffold submit")
	}
	m = pump(t, m, cmd)
	if m.running {
		t.Fatal("expected running false after completion")
	}
	if _, err := os.Stat(filepath.Join(dest, "hello.txt")); err != nil {
		t.Errorf("expected hello.txt rendered by full scaffold: %v", err)
	}
	// After a full scaffold the right pane mirrors the success
	// summary (same two lines runNew prints after q), and the model
	// stays open for the user to quit manually.
	if !strings.Contains(m.output, "done.") {
		t.Fatalf("expected done. in pane, got:\n%s", m.output)
	}
	if !strings.Contains(m.output, "INFO created") {
		t.Fatalf("expected INFO created line in pane, got:\n%s", m.output)
	}
	if !strings.Contains(m.output, "cd "+dest) {
		t.Fatalf("expected cd hint in pane, got:\n%s", m.output)
	}
}

// TestHooksTUI_ModalRendersCanvas verifies the Run/Skip modal renders (via canvas + layered composition) without.
func TestHooksTUI_ModalRendersCanvas(t *testing.T) {
	tpl := writeFixtureTemplate(t)
	resolved := map[string]any{"name": "myapp", "project_name": "myapp"}
	m := newHooksModel(tpl, newTUIStyles(), 100, 30, resolved, context.Background(), t.TempDir(), "fixture", false, false, false)

	m, _ = m.update(keyPress("R"))
	if !m.modalOpen {
		t.Fatal("expected modal open")
	}
	out := m.view().Content
	for _, want := range []string{"fixture", "hooks?", "Run", "Skip", "Cancel"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected modal to contain %q, got:\n%s", want, out)
		}
	}
}

// TestHooksTUI_ModalKeepsRightBorder verifies the run/skip modal does not clip the view pane's right border.
func TestHooksTUI_ModalKeepsRightBorder(t *testing.T) {
	tpl := writeFixtureTemplate(t)
	resolved := map[string]any{"name": "myapp", "project_name": "myapp"}
	m := newHooksModel(tpl, newTUIStyles(), 100, 30, resolved, context.Background(), t.TempDir(), "fixture", false, false, false)

	m, _ = m.update(keyPress("R"))
	if !m.modalOpen {
		t.Fatal("expected modal open")
	}
	out := m.view().Content
	for i, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "│") {
			continue
		}
		// Strip ANSI so colored borders (rendered as ...│\x1b[m) compare
		// correctly, then trim trailing padding before checking the edge.
		plain := strings.TrimRight(ansi.Strip(line), " ")
		if !strings.HasSuffix(plain, "│") {
			t.Fatalf("line %d is missing the right border (clipped by modal canvas):\n%s", i, line)
		}
	}
}

// TestHooksTUI_SkipRunsScaffoldWithoutHooks verifies that choosing Skip
// still scaffolds the project but disables hook execution.
func TestHooksTUI_SkipRunsScaffoldWithoutHooks(t *testing.T) {
	tpl := writeFixtureTemplate(t)
	dest := t.TempDir()
	resolved := map[string]any{"name": "myapp", "project_name": "myapp"}
	m := newHooksModel(tpl, newTUIStyles(), 100, 30, resolved, context.Background(), dest, "myapp", false, false, false)

	m, _ = m.update(keyPress("R"))
	m, cmd := m.update(keyPress("n")) // choose Skip in modal
	if m.modalChoice != 1 {
		t.Fatalf("expected Skip (1) after 'n', got %d", m.modalChoice)
	}
	if !m.didRun {
		t.Fatal("expected didRun after skip submit")
	}
	m = pump(t, m, cmd)
	if strings.Contains(m.output, "pre-hook") {
		t.Fatalf("skip must not run hooks, but output contained pre-hook:\n%s", m.output)
	}
	if _, err := os.Stat(filepath.Join(dest, "hello.txt")); err != nil {
		t.Errorf("expected hello.txt rendered even when skipping hooks: %v", err)
	}
}

// TestHooksTUI_ModalCancelDismisses verifies that choosing Cancel in the
// modal dismisses it without running hooks or scaffolding.
func TestHooksTUI_ModalCancelDismisses(t *testing.T) {
	tpl := writeFixtureTemplate(t)
	dest := t.TempDir()
	resolved := map[string]any{"name": "myapp", "project_name": "myapp"}
	m := newHooksModel(tpl, newTUIStyles(), 100, 30, resolved, context.Background(), dest, "myapp", false, false, false)

	m, _ = m.update(keyPress("R"))
	m, cmd := m.update(keyPress("c")) // choose Cancel in modal
	if m.modalOpen {
		t.Fatal("expected modal closed after cancel")
	}
	if m.didRun {
		t.Fatal("cancel must not set didRun")
	}
	if cmd != nil {
		t.Fatal("cancel must not start a run")
	}
	if _, err := os.Stat(filepath.Join(dest, "hello.txt")); !os.IsNotExist(err) {
		t.Error("cancel must not scaffold _base files")
	}
}
