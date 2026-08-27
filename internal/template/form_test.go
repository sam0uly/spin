package template

import (
	"testing"

	"github.com/sam0uly/spin/internal/params"
)

// TestResolveForm_BuiltinsSeededAndCopiedThrough checks values seeded into ResolveForm flow back out.
func TestResolveForm_BuiltinsSeededAndCopiedThrough(t *testing.T) {
	tpl := &Template{
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{
				"name":  {Type: params.TypeText, Default: "fallback"},
				"color": {Type: params.TypeText, Default: "blue"},
			},
		},
	}
	values := map[string]any{
		"name":         "cli-name",
		"project_name": "cli-name",
	}
	out, err := tpl.ResolveForm(values, false)
	if err != nil {
		t.Fatalf("ResolveForm: %v", err)
	}
	if out["name"] != "cli-name" {
		t.Errorf("name = %v, want cli-name (builtin should override default)", out["name"])
	}
	if out["color"] != "blue" {
		t.Errorf("color = %v, want blue (default)", out["color"])
	}
	if out["project_name"] != "cli-name" {
		t.Errorf("project_name = %v, want cli-name (copied through)", out["project_name"])
	}
}

// TestResolveForm_TemplatedDefault verifies that a param default written as a template string is rendered against.
func TestResolveForm_TemplatedDefault(t *testing.T) {
	tpl := &Template{
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{
				"module": {Type: params.TypeText, Default: "{{ .name }}"},
			},
		},
	}
	out, err := tpl.ResolveForm(map[string]any{"name": "my-app"}, false)
	if err != nil {
		t.Fatalf("ResolveForm: %v", err)
	}
	if out["module"] != "my-app" {
		t.Errorf("module = %v, want my-app (templated default)", out["module"])
	}
}

// TestResolveForm_SelectDefaultOutsideOptions verifies an off-list select default is rejected.
func TestResolveForm_SelectDefaultOutsideOptions(t *testing.T) {
	tpl := &Template{
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{
				"edition": {Type: params.TypeSelect, Options: []string{"2021", "2024"}, Default: "{{ .ed }}"},
			},
		},
	}
	if _, err := tpl.ResolveForm(map[string]any{"ed": "2030"}, false); err == nil {
		t.Fatal("expected error for select default outside options, got nil")
	}
}

// TestResolveForm_SelectDefaultInOptions verifies a valid templated
// select default still resolves cleanly through the same path.
func TestResolveForm_SelectDefaultInOptions(t *testing.T) {
	tpl := &Template{
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{
				"edition": {Type: params.TypeSelect, Options: []string{"2021", "2024"}, Default: "{{ .ed }}"},
			},
		},
	}
	out, err := tpl.ResolveForm(map[string]any{"ed": "2024"}, false)
	if err != nil {
		t.Fatalf("ResolveForm: %v", err)
	}
	if out["edition"] != "2024" {
		t.Errorf("edition = %v, want 2024", out["edition"])
	}
}
