package template

import (
	"strings"
	"testing"

	"github.com/sam0uly/spin/internal/licenses"
	"github.com/sam0uly/spin/internal/params"
)

// TestParseTOML_AllFields verifies the full spin.
func TestParseTOML_AllFields(t *testing.T) {
	input := `
name             = "rust-cli"
description      = "Minimal Rust CLI"
type             = "cli"
language         = "rust"
license          = "MIT"
repository       = "https://github.com/me/rust-cli-template"
min_spin_version = "0.2.0"
exclude          = ["docs/*.md", "CONTRIBUTING.md"]

[author]
name  = "Sam"
email = "sam@example.com"
url   = "https://sam.example.com"

[params]
project_name = "myapp"
edition      = { type = "select", options = ["2021", "2024"], default = "2021" }
feature      = { type = "bool", prompt = "Enable async?", default = true }

[[pre]]
run = "mkdir -p {{.project_name}}/cmd"

[[include]]
path = "ci/**"
if = "{{ .feature }}"

[[post]]
run = "cargo init --name {{.project_name}}"

[[post]]
run = "git init"
`
	st, err := ParseSpinTomlBytes([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if st.Name != "rust-cli" {
		t.Errorf("Name = %q, want %q", st.Name, "rust-cli")
	}
	if st.License != "MIT" {
		t.Errorf("License = %q, want %q", st.License, "MIT")
	}
	if st.Repository != "https://github.com/me/rust-cli-template" {
		t.Errorf("Repository = %q", st.Repository)
	}
	if st.MinSpinVersion != "0.2.0" {
		t.Errorf("MinSpinVersion = %q", st.MinSpinVersion)
	}
	if st.Author.Name != "Sam" || st.Author.Email != "sam@example.com" || st.Author.URL != "https://sam.example.com" {
		t.Errorf("Author = %+v", st.Author)
	}
	if len(st.Exclude) != 2 || st.Exclude[0] != "docs/*.md" || st.Exclude[1] != "CONTRIBUTING.md" {
		t.Errorf("Exclude = %v", st.Exclude)
	}
	if st.Post[0].Run != "cargo init --name {{.project_name}}" {
		t.Errorf("Post[0].Run = %q", st.Post[0].Run)
	}
	if len(st.Post) != 2 {
		t.Errorf("len(Post) = %d, want 2", len(st.Post))
	}
	if len(st.Pre) != 1 || st.Pre[0].Run != "mkdir -p {{.project_name}}/cmd" {
		t.Errorf("Pre = %v", st.Pre)
	}
	if len(st.Include) != 1 || st.Include[0].Path != "ci/**" || st.Include[0].If != "{{ .feature }}" {
		t.Errorf("Include = %v", st.Include)
	}

	// Params: shorthand ("myapp") + inline-table forms.
	pn, ok := st.Params["project_name"]
	if !ok {
		t.Fatal("params.project_name missing")
	}
	if string(pn.Type) != "text" {
		t.Errorf("project_name type = %q, want text", pn.Type)
	}
	if pn.Default != "myapp" {
		t.Errorf("project_name default = %v, want myapp", pn.Default)
	}
	ed, ok := st.Params["edition"]
	if !ok {
		t.Fatal("params.edition missing")
	}
	if string(ed.Type) != "select" {
		t.Errorf("edition type = %q, want select", ed.Type)
	}
	if len(ed.Options) != 2 || ed.Options[0] != "2021" || ed.Options[1] != "2024" {
		t.Errorf("edition options = %v", ed.Options)
	}
	if ed.Default != "2021" {
		t.Errorf("edition default = %v, want 2021", ed.Default)
	}
}

// TestParseTOML_LicenseParamOptionsFilled verifies license params default to the built-in option set.
func TestParseTOML_LicenseParamOptionsFilled(t *testing.T) {
	input := `name = "t"

[params.license]
type = "license"
prompt = "License"
`
	st, err := ParseSpinTomlBytes([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	spec, ok := st.Params["license"]
	if !ok {
		t.Fatal("params.license missing")
	}
	if spec.Type != params.TypeLicense {
		t.Errorf("type = %q, want %q", spec.Type, params.TypeLicense)
	}
	want := licenses.Options()
	if len(spec.Options) != len(want) {
		t.Fatalf("options not filled: got %d, want %d (%v)", len(spec.Options), len(want), spec.Options)
	}
	for i := range want {
		if spec.Options[i] != want[i] {
			t.Errorf("options[%d] = %q, want %q", i, spec.Options[i], want[i])
		}
	}
	// Explicit options are kept as-is.
	input2 := `name = "t"

[params.license]
type = "license"
options = ["MIT"]
`
	st2, err := ParseSpinTomlBytes([]byte(input2))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := st2.Params["license"].Options; len(got) != 1 || got[0] != "MIT" {
		t.Errorf("explicit options must be kept, got %v", got)
	}
}

// TestParseTOML_Minimal confirms a manifest with just name still parses
// -- the new fields are all optional.
func TestParseTOML_Minimal(t *testing.T) {
	input := `name = "minimal"`
	st, err := ParseSpinTomlBytes([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if st.Name != "minimal" {
		t.Errorf("Name = %q", st.Name)
	}
	if st.License != "" || st.Repository != "" {
		t.Errorf("expected empty optional fields, got %+v", st)
	}
}

// TestParseTOML_NameRequired guards the invariant that a manifest
// without a name is rejected.
func TestParseTOML_NameRequired(t *testing.T) {
	_, err := ParseSpinTomlBytes([]byte(`description = "no name field"`))
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("expected name-required error, got %v", err)
	}
}

// TestParseTOML_InvalidTOML confirms the stdlib parser's error
// surfaces with useful context.
func TestParseTOML_InvalidTOML(t *testing.T) {
	_, err := ParseSpinTomlBytes([]byte(`name = "ok"` + "\n" + `this is not = valid toml = "x"`))
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "spin.toml") {
		t.Errorf("error lacks 'spin.toml' prefix: %v", err)
	}
}
