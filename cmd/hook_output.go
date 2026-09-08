package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	tree "charm.land/lipgloss/v2/tree"

	"samouly.fun/spin/internal/theme"
)

// hookSiblings is a dummy two-element Children used to derive lipgloss'
// tree enumerator/indenter glyphs without hardcoding box-drawing chars.
var hookSiblings = tree.NewStringData("a", "b")

// hookIndent is the per-line continuation prefix for streamed hook output.
// It is the pipe mark from lipgloss/tree's DefaultIndenter plus a space.
var hookIndent = strings.TrimRight(tree.DefaultIndenter(hookSiblings, 0), " ") + " "

var hookBodyStyle = lipgloss.NewStyle().Foreground(theme.TextDimmed)

// hookTreeWriter buffers hook stdout/stderr and emits each complete line
// with the tree continuation indent in a dim colour. Headers are emitted
// separately via HookOptions.StepStart; this writer handles body lines.
// A mutex guards the buffer because exec may drive stdout and stderr from
// separate goroutines.
type hookTreeWriter struct {
	emit func(line string) error
	mu   sync.Mutex
	buf  []byte
}

func newHookTreeWriter(emit func(line string) error) *hookTreeWriter {
	return &hookTreeWriter{emit: emit}
}

func (w *hookTreeWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, b...)
	total := len(b)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(w.buf[:idx])
		if err := w.emit(hookBodyStyle.Render(hookIndent+line) + "\n"); err != nil {
			return total, err
		}
		w.buf = w.buf[idx+1:]
	}
	return total, nil
}

func (w *hookTreeWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) == 0 {
		return nil
	}
	err := w.emit(hookBodyStyle.Render(hookIndent+string(w.buf)) + "\n")
	w.buf = nil
	return err
}

// stdoutHookOutput streams hook body lines to os.Stdout.
func stdoutHookOutput() io.Writer {
	return newHookTreeWriter(func(line string) error {
		_, err := fmt.Fprint(os.Stdout, line)
		return err
	})
}

// channelHookOutput forwards hook body lines to ch for the hooks TUI.
func channelHookOutput(ch chan<- string) io.Writer {
	return newHookTreeWriter(func(line string) error {
		ch <- line
		return nil
	})
}

// hookStepHeader renders a styled pre/post hook step header line.
func hookStepHeader(kind, cmd string) string {
	mark := tree.DefaultEnumerator(hookSiblings, 0)
	node := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).
		Render(mark + " " + kind + "-hook")
	return fmt.Sprintf("%s %s\n", node, cmd)
}
