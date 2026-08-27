package template

import (
	"fmt"
	"os"

	"github.com/sam0uly/spin/internal/params"
)

// SpinToml is the parsed manifest at the root of an external template.
//
// Example:
//
//	name            = "rust-cli"
//	description     = "Minimal Rust CLI"
//	type            = "cli"
//	language        = "rust"
//	license         = "MIT"
//	repository      = "https://github.com/me/rust-cli-template"
//	min_spin_version = "0.2.0"
//
//	[author]
//	name  = "Sam"
//	email = "sam@example.com"
//	url   = "https://sam.example.com"
//
//	[params]
//	project_name = { type = "text", prompt = "Project name" }
//	edition      = { type = "select", options = ["2021", "2024"], default = "2021" }
//
//	[[post]]
//	run = "cargo init --name {{.project_name}}"
//
//	[[post]]
//	run = "git init && git add -A"
type SpinToml struct {
	Name           string                 `toml:"name"`
	Description    string                 `toml:"description"`
	Type           string                 `toml:"type"`     // e.g. "tui", "cli"
	Language       string                 `toml:"language"` // e.g. "go", "rust"
	Author         Author                 `toml:"author"`
	License        string                 `toml:"license"`
	Repository     string                 `toml:"repository"`
	MinSpinVersion string                 `toml:"min_spin_version"`
	Exclude        []string               `toml:"exclude"`
	Include        []IncludeRule          `toml:"include"`
	Params         map[string]params.Spec `toml:"params"`
	Pre            []PreStep              `toml:"pre"`
	Post           []PostStep             `toml:"post"`
	Tags           []string               `toml:"tags"`
}

// IncludeRule gates files or directories on a param-driven condition.
// Path is a glob relative to _base/. If is non-empty it is rendered as a
// Go template against the resolved values; the file/directory is included
// only when the result is truthy. An empty If always includes.
type IncludeRule struct {
	Path string `toml:"path"`
	If   string `toml:"if"`
}

// PreStep is one pre-scaffold command. It runs after params resolve
// but before files render, via sh -c in the project root. Steps run
// in order and stop at the first failure.
type PreStep struct {
	Run string `toml:"run"`
}

// Author identifies the template creator. All fields are optional.
type Author struct {
	Name  string `toml:"name"`
	Email string `toml:"email"`
	URL   string `toml:"url"`
}

// PostStep is one post-scaffold command, templated against the
// resolved values and run via sh -c in the project root. Steps run
// in order and stop at the first failure.
type PostStep struct {
	Run string `toml:"run"`
}

// ParseSpinToml reads and parses a spin.toml file from disk.
func ParseSpinToml(path string) (*SpinToml, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSpinTomlBytes(b)
}

// ParseSpinTomlBytes parses a spin.toml document. Name is required.
func ParseSpinTomlBytes(b []byte) (*SpinToml, error) {
	st := &SpinToml{Params: map[string]params.Spec{}}
	if err := parseTOML(b, st); err != nil {
		return nil, err
	}
	if st.Name == "" {
		return nil, fmt.Errorf("spin.toml: name is required")
	}
	return st, nil
}
