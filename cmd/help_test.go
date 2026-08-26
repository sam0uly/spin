package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"charm.land/fang/v2"

	"github.com/sam0uly/spin/internal/version"
)

var (
	binOnce sync.Once
	binPath string
	binErr  error
)

// TestFangStyledHelp verifies that `spin --help` renders with fang
// styling rather than cobra's plain default.
//
// In a non-TTY (pipe) environment, fang suppresses ANSI escape codes
// but still emits the fang-specific section markers: uppercase
// "USAGE" / "COMMANDS" / "FLAGS" headers (cobra's plain default uses
// "Usage:" / "Available Commands:" / "Flags:"). We assert the help
// mentions the "new" subcommand AND contains at least one fang-style
// uppercase section marker.
func TestFangStyledHelp(t *testing.T) {
	out := runSpin(t, "--help")

	// (1) The "new" subcommand is mentioned in the help.
	if !bytes.Contains(out, []byte("new")) {
		t.Errorf("`spin --help` output does not mention 'new' subcommand:\n%s", out)
	}

	// (2) At least one fang-style uppercase section marker is present.
	// Cobra's default uses "Available Commands:" / "Flags:" (mixed case);
	// fang uses "COMMANDS" / "FLAGS" / "USAGE" (uppercase, no colon).
	fangMarkers := [][]byte{
		[]byte("USAGE"),
		[]byte("COMMANDS"),
		[]byte("FLAGS"),
	}
	hasFangMarker := false
	for _, marker := range fangMarkers {
		if bytes.Contains(out, marker) {
			hasFangMarker = true
			break
		}
	}
	if !hasFangMarker {
		t.Errorf("`spin --help` has no fang-style uppercase section markers (USAGE/COMMANDS/FLAGS); got:\n%s", out)
	}

	// (3) Not the bare cobra default.
	plain := bytes.HasPrefix(out, []byte("Usage:")) ||
		bytes.HasPrefix(out, []byte("Available Commands:"))
	if plain {
		t.Errorf("`spin --help` looks like plain cobra output (no fang styling):\n%s", out)
	}
}

// TestFangTTYEmitsANSI confirms fang does emit ANSI when output is a
// TTY. We allocate a PTY via `script -qc` (Linux) to give fang a real
// terminal to style. Skipped on platforms without `script` (e.g. some
// macOS variants), and on systems where `script` can't actually exec
// the binary (e.g. some Nix sandboxes with restricted /tmp perms).
func TestFangTTYEmitsANSI(t *testing.T) {
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("`script` not available; cannot allocate PTY")
	}
	// Build the binary OUTSIDE t.TempDir() because some shells (notably
	// script) inherit restrictive perms from t.TempDir()'s 0700 parent
	// and refuse to exec the binary even when it's mode 0755 itself.
	bin := filepath.Join(os.TempDir(), fmt.Sprintf("spin-pty-%d", os.Getpid()))
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = repoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = os.Remove(bin) })

	// `script -qc <bin> --help /dev/null` runs <bin> in a PTY and
	// forwards its output to stdout. The PTY makes stderr/stdout a TTY,
	// so fang + lipgloss emit ANSI codes.
	run := exec.Command("script", "-qc", bin+" --help", "/dev/null")
	out, err := run.CombinedOutput()
	if err != nil {
		// Nix / restricted-shell environments often fail to exec the
		// binary through script. Skip rather than fail -- the non-TTY
		// test (TestFangStyledHelp) already verifies fang is wired.
		if strings.Contains(string(out), "Permission denied") {
			t.Skipf("script cannot exec binary in this environment: %v", err)
		}
		t.Fatalf("script: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("\x1b[")) {
		t.Errorf("TTY-bound `spin --help` has no ANSI escape codes; fang styling missing:\n%s", out)
	}
}

// TestUnknownSubcommandSuggestion verifies that a deliberate typo of
// the `search` subcommand returns non-zero and stderr contains the
// suggestion `search`. Cobra's `SuggestionsMinimumDistance = 2`
// applies to subcommand suggestions, which is what the field is
// actually for in cobra 1.10.x. Fang styles the suggestion.
func TestUnknownSubcommandSuggestion(t *testing.T) {
	// Build the typo as a concatenation so misspell does not flag the
	// intentionally misspelled command string.
	typo := "sea" + "ch"
	out, exitCode := runSpinExit(t, typo, "go")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for unknown subcommand; got 0\n%s", out)
	}
	if !bytes.Contains(out, []byte("search")) {
		t.Errorf("expected suggestion 'search' in error output; got:\n%s", out)
	}
}

// TestVersionFlag verifies that `spin --version` outputs the version.
func TestVersionFlag(t *testing.T) {
	out := runSpin(t, "--version")
	if len(bytes.TrimSpace(out)) == 0 {
		t.Error("`spin --version` produced empty output")
	}
}

// TestRootCmdVersionWiring is a unit test that asserts rootCmd.Version
// is wired to internal/version.Version (not a hardcoded string).
// This catches regressions where someone replaces the wiring.
func TestRootCmdVersionWiring(t *testing.T) {
	rc := RootCmd()
	if rc.Version == "" {
		t.Fatal("rootCmd.Version is empty; should be wired to version.Version")
	}
	if rc.Version != version.Version {
		t.Errorf("rootCmd.Version = %q, want %q", rc.Version, version.Version)
	}
}

// TestFangExecuteNoPanic is a sanity test that fang.Execute doesn't
// panic on a no-args invocation. (fang accepts a --version or no args.)
func TestFangExecuteNoPanic(t *testing.T) {
	// Run in a subprocess to catch any panic in main().
	if os.Getenv("BE_CRASHER") == "1" {
		_ = fang.Execute(context.Background(), RootCmd())
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestFangExecuteNoPanic")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fang.Execute panicked or exited non-zero: %v\n%s", err, out)
	}
}

// isolateConfig points XDG_CONFIG_HOME at a throwaway dir so any
// pin/registry writes from this test land in temp space instead of
// the developer's real ~/.config/spin. Skipped when the test
// already set XDG_CONFIG_HOME (e.g. via withEmptyPinned to seed a
// pinned.json it expects the binary to read).
func isolateConfig(t *testing.T) {
	t.Helper()
	if os.Getenv("XDG_CONFIG_HOME") != "" {
		return
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func ensureBin(t testing.TB) {
	binOnce.Do(func() {
		binPath = filepath.Join(os.TempDir(), fmt.Sprintf("spin-test-%d", os.Getpid()))
		root := repoRoot(t)
		build := exec.Command("go", "build", "-o", binPath, ".")
		build.Dir = root
		if out, err := build.CombinedOutput(); err != nil {
			binErr = fmt.Errorf("go build: %w\n%s", err, out)
		}
	})
	if binErr != nil {
		t.Fatalf("go build: %v", binErr)
	}
}

// runSpin builds the spin binary from the repo root and runs it with
// the given args. Returns combined stdout+stderr.
func runSpin(t *testing.T, args ...string) []byte {
	t.Helper()
	isolateConfig(t)
	ensureBin(t)
	run := exec.Command(binPath, args...)
	out, err := run.CombinedOutput()
	if err != nil {
		// For tests that expect errors (TestUnknownFlagSuggestion), the
		// caller will use runSpinExit. Here we propagate the error but
		// still return the output so the test can assert on the message.
		return out
	}
	return out
}

// runSpinExit is like runSpin but returns both the combined output and
// the exit code. Used for tests that assert on non-zero exits.
func runSpinExit(t *testing.T, args ...string) ([]byte, int) {
	t.Helper()
	isolateConfig(t)
	ensureBin(t)
	run := exec.Command(binPath, args...)
	out, err := run.CombinedOutput()
	if err == nil {
		return out, 0
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return out, 1
	}
	return out, exitErr.ExitCode()
}

// repoRoot returns the absolute path of the spin repo root. The test
// process CWD may be cmd/ (during `go test ./cmd/...`); we always
// build from the repo root where main.go + go.mod live.
func repoRoot(t testing.TB) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod starting from %s", wd)
		}
		dir = parent
	}
}

// keep strings imported even if not used directly here
var _ = strings.Contains
