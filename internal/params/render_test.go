package params

import (
	"testing"
)

// TestRenderStr verifies that prompt/default strings are rendered as Go templates against the available values:.
func TestRenderStr(t *testing.T) {
	values := map[string]any{
		"name":         "my-app",
		"project_name": "my-app",
	}
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"plain prompt", "plain prompt"},
		{"Name for {{ .name }}", "Name for my-app"},
		{"{{ .name }}", "my-app"},
		{"{{ .name | upper }}", "MY-APP"},
		{"{{ snake_case .name }}", "my_app"},
		{"{{ kebab .name }}", "my-app"},
		{"{{ .name | trim }}", "my-app"},
		// malformed template -> safe fallback to the raw string.
		{"oops {{ .name", "oops {{ .name"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := renderStr(tc.in, values); got != tc.want {
				t.Errorf("renderStr(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestTextParam_TemplatedDefault verifies that a text param whose default is a template string is resolved.
func TestTextParam_TemplatedDefault(t *testing.T) {
	p := NewText("module", "Module name", "{{ .name }}")
	p.SetDefault(map[string]any{"name": "my-app"})
	if got := p.Value().String; got != "my-app" {
		t.Errorf("templated default = %q, want %q", got, "my-app")
	}
}

// TestSelectParam_TemplatedDefault verifies the same templating for a
// select param's default.
func TestSelectParam_TemplatedDefault(t *testing.T) {
	p := NewSelect("edition", "Edition", []string{"2021", "2024"}, "{{ .ed }}")
	p.SetDefault(map[string]any{"ed": "2024"})
	if got := p.Value().String; got != "2024" {
		t.Errorf("templated default = %q, want %q", got, "2024")
	}
}

// TestFromAny verifies the value coercion used when seeding builtins
// (name, project_name) onto params and by ResolveForm.
func TestFromAny(t *testing.T) {
	cases := []struct {
		in   any
		want Value
	}{
		{"s", Value{Kind: TypeText, String: "s"}},
		{42, Value{Kind: TypeNumber, Int: 42}},
		{true, Value{Kind: TypeBool, Bool: true}},
		{[]string{"a", "b"}, Value{Kind: TypeMultiSelect, List: []string{"a", "b"}}},
		{[]any{"x", "y"}, Value{Kind: TypeMultiSelect, List: []string{"x", "y"}}},
	}
	for _, tc := range cases {
		got := FromAny(tc.in)
		if got.Kind != tc.want.Kind {
			t.Errorf("FromAny(%v).Kind = %q, want %q", tc.in, got.Kind, tc.want.Kind)
		}
	}
}
