package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestInit_CreatesBaseTree verifies `spin init` writes
// spin.toml, _base/file.txt.tmpl, and README.md into <name>/.
// The template body should contain the resolved name; the
// README should link to the spin repo.
func TestInit_CreatesBaseTree(t *testing.T) {
	dir := t.TempDir()
	out, exitCode := runSpinWithDir(t, dir, "init", "my-template")
	if exitCode != 0 {
		t.Fatalf("expected exit 0; got %d\n%s", exitCode, out)
	}
	dest := filepath.Join(dir, "my-template")
	for _, want := range []string{
		filepath.Join(dest, "spin.toml"),
		filepath.Join(dest, "_base", "file.txt.tmpl"),
		filepath.Join(dest, "README.md"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("expected %s to exist; got: %v", want, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(dest, "spin.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`name = "my-template"`)) {
		t.Errorf("spin.toml should contain name = \"my-template\"; got:\n%s", b)
	}
	if !bytes.Contains(b, []byte("[params]")) {
		t.Errorf("spin.toml should have a [params] section; got:\n%s", b)
	}
	if !bytes.Contains(b, []byte("[[post]]")) {
		t.Errorf("spin.toml should have a [[post]] example; got:\n%s", b)
	}
}

// TestInit_RejectsExistingDir verifies we don't silently
// overwrite an existing directory. A typo or a stray
// `spin init template` over real work is a hostile UX.
func TestInit_RejectsExistingDir(t *testing.T) {
	dir := t.TempDir()
	// Pre-create the dest dir.
	if err := os.MkdirAll(filepath.Join(dir, "collide"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "collide", "marker.txt"), []byte("important"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, exitCode := runSpinWithDir(t, dir, "init", "collide")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit; got 0\n%s", out)
	}
	if !bytes.Contains(out, []byte("already exists")) {
		t.Errorf("error should mention 'already exists'; got:\n%s", out)
	}
	// The marker file must still be intact.
	b, err := os.ReadFile(filepath.Join(dir, "collide", "marker.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "important" {
		t.Errorf("existing file was modified; got: %q", b)
	}
}

// TestInit_DirFlag verifies --dir puts the new template in the
// requested parent rather than the CWD.
func TestInit_DirFlag(t *testing.T) {
	cwd := t.TempDir()
	parent := t.TempDir()
	out, exitCode := runSpinWithDir(t, cwd, "init", "x", "--dir", parent)
	if exitCode != 0 {
		t.Fatalf("expected exit 0; got %d\n%s", exitCode, out)
	}
	if _, err := os.Stat(filepath.Join(parent, "x", "spin.toml")); err != nil {
		t.Errorf("expected --dir to put template at %s/x/spin.toml; got: %v", parent, err)
	}
	// And NOT in cwd.
	if _, err := os.Stat(filepath.Join(cwd, "x", "spin.toml")); !os.IsNotExist(err) {
		t.Errorf("--dir should NOT create template in cwd; got: %v", err)
	}
}

// TestInit_RejectsBadName verifies path-separator and dot names
// are rejected, since they would let the user create templates
// outside the intended parent.
func TestInit_RejectsBadName(t *testing.T) {
	cases := []string{"", ".", "..", "with/slash", "with\\backslash", "x\x00y"}
	dir := t.TempDir()
	for _, name := range cases {
		t.Run("name="+name, func(t *testing.T) {
			out, exitCode := runSpinWithDir(t, dir, "init", name)
			if exitCode == 0 {
				t.Errorf("expected non-zero exit for bad name %q; got 0\n%s", name, out)
			}
		})
	}
}

// TestInit_FixtureIsRenderable verifies the produced template
// can be rendered end-to-end by `spin new`. We render it into
// a temp dir and check the .tmpl file came out with the project
// name substituted.
func TestInit_FixtureIsRenderable(t *testing.T) {
	dir := t.TempDir()
	if out, code := runSpinWithDir(t, dir, "init", "rt"); code != 0 {
		t.Fatalf("init: %d\n%s", code, out)
	}
	outDir := t.TempDir()
	out, code := runSpinWithDir(t, dir, "new", "myapp",
		"--template", filepath.Join(dir, "rt"),
		"--param", "license=MIT",
		"--dest", outDir,
	)
	if code != 0 {
		t.Fatalf("new: %d\n%s", code, out)
	}
	rendered, err := os.ReadFile(filepath.Join(outDir, "file.txt"))
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}
	if !bytes.Contains(rendered, []byte("# myapp")) {
		t.Errorf("rendered file should contain '# myapp' header; got:\n%s", rendered)
	}
	// And the spin.toml sentinel value should NOT have leaked
	// through (TPL-16).
	if _, err := os.Stat(filepath.Join(outDir, "spin.toml")); !os.IsNotExist(err) {
		t.Errorf("rendered project should not contain spin.toml (TPL-16); stat err=%v", err)
	}
	// Built-in licensing: the fixture's `type = "license"` param with
	// --param license=MIT must produce a LICENSE file with the year.
	lic, err := os.ReadFile(filepath.Join(outDir, "LICENSE"))
	if err != nil {
		t.Fatalf("expected generated LICENSE: %v", err)
	}
	if !bytes.Contains(lic, []byte("MIT License")) {
		t.Errorf("LICENSE should be the MIT text; got:\n%s", lic)
	}
}

// TestInit_HelpText verifies the help mentions --dir and a
// brief example, so users can discover the flag.
func TestInit_HelpText(t *testing.T) {
	out := runSpin(t, "init", "--help")
	if !bytes.Contains(out, []byte("--dir")) {
		t.Errorf("`spin init --help` should mention --dir; got:\n%s", out)
	}
	if !bytes.Contains(out, []byte("init")) {
		t.Errorf("`spin init --help` should mention 'init'; got:\n%s", out)
	}
	if !strings.Contains(string(out), "spin.toml") {
		t.Errorf("`spin init --help` should mention the spin.toml manifest; got:\n%s", out)
	}
}

// runSpinWithDir runs the spin binary in the given working
// directory. Used by init tests so we can isolate the
// destination from the test process's CWD.
func runSpinWithDir(t *testing.T, dir string, args ...string) ([]byte, int) {
	t.Helper()
	ensureBin(t)
	run := exec.Command(binPath, args...)
	run.Dir = dir
	out, err := run.CombinedOutput()
	if err == nil {
		return out, 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return out, exitErr.ExitCode()
	}
	return out, 1
}
