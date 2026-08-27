package cmd

import (
	"slices"
	"strings"
	"testing"

	"github.com/sam0uly/spin/internal/licenses"
	"github.com/sam0uly/spin/internal/params"
	"github.com/sam0uly/spin/internal/template"
)

// tplWithParams builds an in-memory *template.Template with the given params pre-loaded.
func tplWithParams(t *testing.T, specs map[string]params.Spec) *template.Template {
	t.Helper()
	return &template.Template{
		Name:     "test",
		SpinToml: &template.SpinToml{Params: specs},
	}
}

func TestSplitParamEntry(t *testing.T) {
	cases := []struct {
		in       string
		wantKey  string
		wantVal  string
		wantErr  bool
		errMatch string
	}{
		{"port=8080", "port", "8080", false, ""},
		{"name=hello world", "name", "hello world", false, ""},
		{"name = spaced", "name", " spaced", false, ""}, // value not trimmed, key is
		{"port=", "port", "", false, ""},                // empty value still parses; ResolveForm applies default if needed
		{"=8080", "", "", true, "empty key"},
		{"nokey", "", "", true, "missing '='"},
		{"", "", "", true, "missing '='"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			k, v, err := splitParamEntry(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (k=%q v=%q)", tc.errMatch, k, v)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if k != tc.wantKey {
				t.Errorf("key = %q, want %q", k, tc.wantKey)
			}
			if v != tc.wantVal {
				t.Errorf("value = %q, want %q", v, tc.wantVal)
			}
		})
	}
}

func TestCoerceParamValue_Number(t *testing.T) {
	min, max := 1, 65535
	spec := params.Spec{Type: params.TypeNumber, Min: &min, Max: &max}
	cases := []struct {
		in      string
		want    any
		wantErr bool
		errFrag string
	}{
		{"8080", 8080, false, ""},
		{"1", 1, false, ""}, // boundary
		{"65535", 65535, false, ""},
		{"-1", nil, true, "below min"},
		{"65536", nil, true, "above max"},
		{"notanumber", nil, true, "expected integer"},
		{"", nil, true, "expected integer"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := coerceParamValue(spec, tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (got=%v)", tc.errFrag, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v (%T), want %v", got, got, tc.want)
			}
		})
	}
}

func TestCoerceParamValue_Bool(t *testing.T) {
	spec := params.Spec{Type: params.TypeBool}
	cases := []struct {
		in   string
		want bool
		err  bool
	}{
		{"true", true, false},
		{"True", true, false},
		{"TRUE", true, false},
		{"1", true, false},
		{"yes", true, false},
		{"y", true, false},
		{"on", true, false},
		{"false", false, false},
		{"0", false, false},
		{"no", false, false},
		{"off", false, false},
		{"", false, true},
		{"maybe", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := coerceParamValue(spec, tc.in)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCoerceParamValue_MultiSelect(t *testing.T) {
	spec := params.Spec{Type: params.TypeMultiSelect}
	cases := []struct {
		in   string
		want []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b , c", []string{"a", "b", "c"}}, // spaces trimmed
		{"a,,b", []string{"a", "b"}},          // empties dropped
		{",a,", []string{"a"}},                // leading/trailing comma handled
		{"single", []string{"single"}},
		{"", []string{}}, // "" split = [""], all trimmed out => empty slice
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := coerceParamValue(spec, tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", got)
				}
				return
			}
			gotSlice, ok := got.([]string)
			if !ok {
				t.Fatalf("got type %T, want []string", got)
			}
			if len(gotSlice) != len(tc.want) {
				t.Fatalf("got %v, want %v", gotSlice, tc.want)
			}
			for i := range gotSlice {
				if gotSlice[i] != tc.want[i] {
					t.Errorf("[%d] got %q, want %q", i, gotSlice[i], tc.want[i])
				}
			}
		})
	}
}

func TestCoerceParamValue_Select(t *testing.T) {
	spec := params.Spec{Type: params.TypeSelect, Options: []string{"red", "green", "blue"}}
	if _, err := coerceParamValue(spec, "red"); err != nil {
		t.Errorf("red should be a valid option: %v", err)
	}
	if _, err := coerceParamValue(spec, "yellow"); err == nil {
		t.Error("yellow should be rejected")
	}
	// No Options list: any value passes (template author's choice).
	openSpec := params.Spec{Type: params.TypeSelect}
	if _, err := coerceParamValue(openSpec, "anything"); err != nil {
		t.Errorf("open select should accept anything: %v", err)
	}
}

func TestCoerceParamValue_License(t *testing.T) {
	spec := params.Spec{Type: params.TypeLicense, Options: []string{"MIT", "Apache-2.0", "None"}}
	got, err := coerceParamValue(spec, "MIT")
	if err != nil {
		t.Fatalf("MIT should be a valid license: %v", err)
	}
	if got != "MIT" {
		t.Errorf("got %v, want %q", got, "MIT")
	}
	if _, err := coerceParamValue(spec, "GPL-3.0-only"); err == nil {
		t.Error("out-of-options license should be rejected")
	}
	// Auto-filled options (specFromMap) make any built-in license valid.
	filled := params.Spec{Type: params.TypeLicense, Options: licenses.Options()}
	if _, err := coerceParamValue(filled, "CC0-1.0"); err != nil {
		t.Errorf("built-in license should be accepted: %v", err)
	}
}

func TestCoerceParamValue_TextAndStringish(t *testing.T) {
	for _, typ := range []params.Type{params.TypeText, params.TypeTextarea, params.TypePath, params.TypeSecret, ""} {
		spec := params.Spec{Type: typ}
		got, err := coerceParamValue(spec, "hello world / with / slashes")
		if err != nil {
			t.Errorf("type=%s: unexpected error: %v", typ, err)
		}
		if got != "hello world / with / slashes" {
			t.Errorf("type=%s: got %q, want %q", typ, got, "hello world / with / slashes")
		}
	}
}

// TestApplyParamFlags_Happy verifies the full path: known keys,
// right types, returned map contains the typed values.
func TestApplyParamFlags_Happy(t *testing.T) {
	tpl := tplWithParams(t, map[string]params.Spec{
		"port":     {Type: params.TypeNumber},
		"verbose":  {Type: params.TypeBool},
		"features": {Type: params.TypeMultiSelect},
		"name":     {Type: params.TypeText},
	})
	got, err := applyParamFlags(tpl, []string{
		"port=8080",
		"verbose=true",
		"features=ci,release",
		"name=myapp",
	})
	if err != nil {
		t.Fatalf("applyParamFlags: %v", err)
	}
	if got["port"] != 8080 {
		t.Errorf("port = %v, want 8080", got["port"])
	}
	if got["verbose"] != true {
		t.Errorf("verbose = %v, want true", got["verbose"])
	}
	if !slices.Equal(got["features"].([]string), []string{"ci", "release"}) {
		t.Errorf("features = %v, want [ci release]", got["features"])
	}
	if got["name"] != "myapp" {
		t.Errorf("name = %v, want myapp", got["name"])
	}
}

// TestApplyParamFlags_UnknownKey names both the offending entry and known keys.
func TestApplyParamFlags_UnknownKey(t *testing.T) {
	tpl := tplWithParams(t, map[string]params.Spec{
		"port": {Type: params.TypeNumber},
	})
	_, err := applyParamFlags(tpl, []string{"prt=8080"})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown key") {
		t.Errorf("error should mention 'unknown key'; got: %s", msg)
	}
	if !strings.Contains(msg, "port") {
		t.Errorf("error should list known key 'port'; got: %s", msg)
	}
}

// TestApplyParamFlags_MalformedEntry verifies `--param=foo` (no `=`) and `--param==8080` (empty key) produce.
func TestApplyParamFlags_MalformedEntry(t *testing.T) {
	tpl := tplWithParams(t, map[string]params.Spec{
		"port": {Type: params.TypeNumber},
	})
	_, err := applyParamFlags(tpl, []string{"foo"})
	if err == nil {
		t.Fatal("expected error for missing '='")
	}
	_, err = applyParamFlags(tpl, []string{"=8080"})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

// TestApplyParamFlags_NumberOutOfRange verifies the per-spec min/ max bounds from spin.
func TestApplyParamFlags_NumberOutOfRange(t *testing.T) {
	min, max := 1, 10
	tpl := tplWithParams(t, map[string]params.Spec{
		"port": {Type: params.TypeNumber, Min: &min, Max: &max},
	})
	if _, err := applyParamFlags(tpl, []string{"port=999"}); err == nil {
		t.Error("port=999 should fail max=10 check")
	}
	if _, err := applyParamFlags(tpl, []string{"port=0"}); err == nil {
		t.Error("port=0 should fail min=1 check")
	}
	if _, err := applyParamFlags(tpl, []string{"port=5"}); err != nil {
		t.Errorf("port=5 should succeed: %v", err)
	}
}

// TestApplyParamFlags_BadBoolType verifies that `--param foo=42` on a bool param errors with the same message.
func TestApplyParamFlags_BadBoolType(t *testing.T) {
	tpl := tplWithParams(t, map[string]params.Spec{
		"verbose": {Type: params.TypeBool},
	})
	_, err := applyParamFlags(tpl, []string{"verbose=42"})
	if err == nil {
		t.Fatal("expected error coercing 42 to bool")
	}
	if !strings.Contains(err.Error(), "bool") {
		t.Errorf("error should mention 'bool'; got: %s", err.Error())
	}
}

// TestJoinKnownParams verifies the unknown-key error message
// includes a stable, alphabetical list of the known keys.
func TestJoinKnownParams(t *testing.T) {
	specs := map[string]params.Spec{
		"zeta":  {},
		"alpha": {},
		"mu":    {},
	}
	got := joinKnownParams(specs)
	want := "alpha, mu, zeta"
	if got != want {
		t.Errorf("joinKnownParams = %q, want %q", got, want)
	}
}

// TestParamFlag_HelpText verifies the flag's Usage string appears in `spin new --help`, so users can discover.
func TestParamFlag_HelpText(t *testing.T) {
	out := runSpin(t, "new", "--help")
	if !strings.Contains(string(out), "--param") {
		t.Errorf("`spin new --help` should document --param flag; got:\n%s", out)
	}
	if !strings.Contains(string(out), "key=value") {
		t.Errorf("`spin new --help` should show --param format 'key=value'; got:\n%s", out)
	}
}
