package params

import (
	"reflect"
	"testing"
)

// TestValue_RoundTrip verifies that for every param type, building a Value, calling Apply(), and reading Value()
func TestValue_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		p    Param
		in   Value
	}{
		{"text", NewText("n", "p", "def"), Value{Kind: TypeText, String: "hello"}},
		{"textarea", NewTextarea("n", "p", "def"), Value{Kind: TypeTextarea, String: "line1\nline2"}},
		{"number", NewNumber("n", "p", 0, nil, nil), Value{Kind: TypeNumber, Int: 42}},
		{"bool", NewBool("n", "p", false), Value{Kind: TypeBool, Bool: true}},
		{"path", NewPath("n", "p", "def"), Value{Kind: TypePath, Path: "/tmp"}},
		{"secret", NewSecret("n", "p"), Value{Kind: TypeSecret, String: "shh"}},
		{"select", NewSelect("n", "p", []string{"a", "b"}, "a"), Value{Kind: TypeSelect, String: "b"}},
		{"multiselect", NewMultiSelect("n", "p", []string{"a", "b"}, nil), Value{Kind: TypeMultiSelect, List: []string{"a", "b"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.p.Apply(tc.in)
			got := tc.p.Value()
			if !reflect.DeepEqual(got, tc.in) {
				t.Errorf("round-trip: got %+v, want %+v", got, tc.in)
			}
		})
	}
}

// TestOrPrompt_FallsBackToName verifies the orPrompt helper returns the prompt when set, else the name.
func TestOrPrompt_FallsBackToName(t *testing.T) {
	cases := []struct {
		name, prompt, want string
	}{
		{"project_name", "Project name?", "Project name?"},
		{"project_name", "", "project_name"},
		{"port", "Port number", "Port number"},
	}
	for _, tc := range cases {
		got := orPrompt(tc.name, tc.prompt)
		if got != tc.want {
			t.Errorf("orPrompt(%q, %q) = %q, want %q", tc.name, tc.prompt, got, tc.want)
		}
	}
}

// TestTextParam_DefaultEmptyString verifies empty defaults leave the placeholder unset.
func TestTextParam_DefaultEmptyString(t *testing.T) {
	tp := NewText("n", "p", "")
	if tp.Default() != "" {
		t.Errorf("Default = %v, want empty", tp.Default())
	}
	tp.Apply(Value{String: "user-value"})
	if tp.Value().String != "user-value" {
		t.Errorf("Value().String = %q, want %q", tp.Value().String, "user-value")
	}
}

// TestNumberParam_DefaultZero verifies the number param's default-zero round-trip: a default of 0 with no min/max.
func TestNumberParam_DefaultZero(t *testing.T) {
	np := NewNumber("port", "Port", 0, nil, nil)
	if np.Default() != 0 {
		t.Errorf("Default = %v, want 0", np.Default())
	}
	np.Apply(Value{Int: 8080})
	if np.Value().Int != 8080 {
		t.Errorf("Value().Int = %d, want 8080", np.Value().Int)
	}
}

// TestMultiSelectParam_DefaultNil verifies a nil default doesn't panic; the form should just open with no options.
func TestMultiSelectParam_DefaultNil(t *testing.T) {
	mp := NewMultiSelect("features", "Features", []string{"a", "b"}, nil)
	if d, ok := mp.Default().([]string); !ok || len(d) != 0 {
		t.Errorf("Default = %v, want empty []string (or nil)", mp.Default())
	}
	mp.Apply(Value{List: []string{"a"}})
	if !reflect.DeepEqual(mp.Value().List, []string{"a"}) {
		t.Errorf("Value().List = %v, want [a]", mp.Value().List)
	}
}
