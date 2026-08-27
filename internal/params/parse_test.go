package params

import (
	"reflect"
	"sort"
	"testing"
)

// TestParse_Text verifies a `{name: {type: "text", default: "..."}}`
// entry produces a *TextParam with the right defaults.
func TestParse_Text(t *testing.T) {
	specs := SpecMap{
		"project_name": {Type: TypeText, Default: "hello"},
	}
	ps, err := Parse(specs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(ps) != 1 {
		t.Fatalf("got %d params, want 1", len(ps))
	}
	tp, ok := ps[0].(*TextParam)
	if !ok {
		t.Fatalf("ps[0] is %T, want *TextParam", ps[0])
	}
	if tp.def != "hello" {
		t.Errorf("def = %q, want %q", tp.def, "hello")
	}
	if tp.name != "project_name" {
		t.Errorf("name = %q, want %q", tp.name, "project_name")
	}
}

// TestParse_Textarea verifies the textarea type produces a
// *TextareaParam with multi-line default preserved.
func TestParse_Textarea(t *testing.T) {
	specs := SpecMap{
		"readme": {Type: TypeTextarea, Default: "line1\nline2"},
	}
	ps, err := Parse(specs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tp, ok := ps[0].(*TextareaParam)
	if !ok {
		t.Fatalf("ps[0] is %T, want *TextareaParam", ps[0])
	}
	if tp.def != "line1\nline2" {
		t.Errorf("def = %q, want %q (newlines must be preserved)", tp.def, "line1\nline2")
	}
}

// TestParse_Number verifies number bounds (min/max) are preserved on the NumberParam.
func TestParse_Number(t *testing.T) {
	min, max := 0, 100
	specs := SpecMap{
		"port": {Type: TypeNumber, Default: 42, Min: &min, Max: &max},
	}
	ps, err := Parse(specs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	np, ok := ps[0].(*NumberParam)
	if !ok {
		t.Fatalf("ps[0] is %T, want *NumberParam", ps[0])
	}
	if np.def != 42 {
		t.Errorf("def = %d, want 42", np.def)
	}
	if np.min == nil || *np.min != 0 {
		t.Errorf("min = %v, want pointer to 0", np.min)
	}
	if np.max == nil || *np.max != 100 {
		t.Errorf("max = %v, want pointer to 100", np.max)
	}
}

// TestParse_Select verifies select options are preserved in order. Order matters: huh.
func TestParse_Select(t *testing.T) {
	specs := SpecMap{
		"edition": {Type: TypeSelect, Options: []string{"a", "b", "c"}, Default: "a"},
	}
	ps, err := Parse(specs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sp, ok := ps[0].(*SelectParam)
	if !ok {
		t.Fatalf("ps[0] is %T, want *SelectParam", ps[0])
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(sp.options, want) {
		t.Errorf("options = %v, want %v", sp.options, want)
	}
	if sp.def != "a" {
		t.Errorf("def = %q, want %q", sp.def, "a")
	}
}

// TestParse_MultiSelect verifies multi-select default selection (a single option by default; multiple supported).
func TestParse_MultiSelect(t *testing.T) {
	specs := SpecMap{
		"features": {Type: TypeMultiSelect, Options: []string{"a", "b"}, Default: []string{"a"}},
	}
	ps, err := Parse(specs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mp, ok := ps[0].(*MultiSelectParam)
	if !ok {
		t.Fatalf("ps[0] is %T, want *MultiSelectParam", ps[0])
	}
	if !reflect.DeepEqual(mp.def, []string{"a"}) {
		t.Errorf("def = %v, want [a]", mp.def)
	}
}

// TestParse_Bool verifies bool default is preserved.
func TestParse_Bool(t *testing.T) {
	specs := SpecMap{
		"verbose": {Type: TypeBool, Default: true},
	}
	ps, err := Parse(specs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	bp, ok := ps[0].(*BoolParam)
	if !ok {
		t.Fatalf("ps[0] is %T, want *BoolParam", ps[0])
	}
	if bp.def != true {
		t.Errorf("def = %v, want true", bp.def)
	}
}

// TestParse_Path verifies path params have a string default
// (the huh FilePicker's starting directory / filename).
func TestParse_Path(t *testing.T) {
	specs := SpecMap{
		"output": {Type: TypePath, Default: "/tmp"},
	}
	ps, err := Parse(specs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	pp, ok := ps[0].(*PathParam)
	if !ok {
		t.Fatalf("ps[0] is %T, want *PathParam", ps[0])
	}
	if pp.def != "/tmp" {
		t.Errorf("def = %q, want %q", pp.def, "/tmp")
	}
}

// TestParse_Secret verifies secret params parse without a default.
func TestParse_Secret(t *testing.T) {
	specs := SpecMap{
		"api_key": {Type: TypeSecret},
	}
	ps, err := Parse(specs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sp, ok := ps[0].(*SecretParam)
	if !ok {
		t.Fatalf("ps[0] is %T, want *SecretParam", ps[0])
	}
	if sp.def != "" {
		t.Errorf("def = %q, want empty (secrets have no default)", sp.def)
	}
}

// TestParse_UnknownType verifies the parser returns ErrUnknownType for an unrecognised type string.
func TestParse_UnknownType(t *testing.T) {
	specs := SpecMap{
		"weird": {Type: Type("madeup")},
	}
	_, err := Parse(specs)
	if err == nil {
		t.Fatal("Parse should return error for unknown type")
	}
	eu, ok := err.(ErrUnknownType)
	if !ok {
		t.Fatalf("err is %T, want ErrUnknownType", err)
	}
	if eu.Name != "weird" {
		t.Errorf("ErrUnknownType.Name = %q, want %q", eu.Name, "weird")
	}
	if eu.Type != Type("madeup") {
		t.Errorf("ErrUnknownType.Type = %q, want %q", eu.Type, "madeup")
	}
}

// TestParse_Shorthand verifies the shorthand form (no inline table, just a string value) produces a *TextParam.
func TestParse_Shorthand(t *testing.T) {
	// The shorthand is detected at the TOML layer (parseTOML in
	// internal/template); for parse_test, we hand-craft the
	// equivalent Spec the shorthand would produce.
	specs := SpecMap{
		"project_name": {Type: TypeText, Default: "myapp"},
	}
	ps, err := Parse(specs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tp, ok := ps[0].(*TextParam)
	if !ok {
		t.Fatalf("ps[0] is %T, want *TextParam (shorthand → text)", ps[0])
	}
	if tp.def != "myapp" {
		t.Errorf("def = %q, want %q", tp.def, "myapp")
	}
}

// TestSetDefaults verifies SetDefaults applies each param's default, the non-interactive path.
func TestSetDefaults(t *testing.T) {
	specs := SpecMap{
		"name":  {Type: TypeText, Default: "myapp"},
		"port":  {Type: TypeNumber, Default: 8080},
		"debug": {Type: TypeBool, Default: true},
		"ed":    {Type: TypeSelect, Options: []string{"2021", "2024"}, Default: "2021"},
	}
	ps, err := Parse(specs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	SetDefaults(ps, map[string]any{})

	want := map[string]Value{
		"name":  {Kind: TypeText, String: "myapp"},
		"port":  {Kind: TypeNumber, Int: 8080},
		"debug": {Kind: TypeBool, Bool: true},
		"ed":    {Kind: TypeSelect, String: "2021"},
	}
	for _, p := range ps {
		w, ok := want[p.Name()]
		if !ok {
			t.Errorf("unexpected param %q", p.Name())
			continue
		}
		got := p.Value()
		if !reflect.DeepEqual(got, w) {
			t.Errorf("%s Value() = %+v, want %+v", p.Name(), got, w)
		}
	}
}

// TestSetDefaults_PreservesOrder is a smoke test that SetDefaults does not re-order the param slice (some.
func TestSetDefaults_PreservesOrder(t *testing.T) {
	specs := SpecMap{
		"a": {Type: TypeText, Default: "A"},
		"b": {Type: TypeText, Default: "B"},
		"c": {Type: TypeText, Default: "C"},
	}
	ps, err := Parse(specs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	names := []string{ps[0].Name(), ps[1].Name(), ps[2].Name()}
	SetDefaults(ps, map[string]any{})
	got := []string{ps[0].Name(), ps[1].Name(), ps[2].Name()}
	if !reflect.DeepEqual(names, got) {
		t.Errorf("order changed: %v → %v", names, got)
	}
	// And the values are independent -- sort by name and check.
	sort.Strings(names)
	sort.Strings(got)
	if !reflect.DeepEqual(names, got) {
		t.Errorf("names mismatch: %v vs %v", names, got)
	}
}

// TestSetDefault_Textarea verifies SetDefault on a textarea param
// applies the default string via Value().
func TestSetDefault_Textarea(t *testing.T) {
	p := NewTextarea("desc", "Description", "a\nb")
	p.SetDefault(map[string]any{})
	got := p.Value()
	want := Value{Kind: TypeTextarea, String: "a\nb"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Value() = %+v, want %+v", got, want)
	}
}

// TestSetDefault_MultiSelect verifies SetDefault on a multiselect
// param applies the default []string via Value().
func TestSetDefault_MultiSelect(t *testing.T) {
	p := NewMultiSelect("features", "Features", []string{"a", "b"}, []string{"a"})
	p.SetDefault(map[string]any{})
	got := p.Value()
	want := Value{Kind: TypeMultiSelect, List: []string{"a"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Value() = %+v, want %+v", got, want)
	}
}

// TestSetDefault_Path verifies SetDefault on a path param
// applies the default string via Value().
func TestSetDefault_Path(t *testing.T) {
	p := NewPath("out", "Output", "/tmp")
	p.SetDefault(map[string]any{})
	got := p.Value()
	want := Value{Kind: TypePath, Path: "/tmp"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Value() = %+v, want %+v", got, want)
	}
}

// TestSetDefault_Secret verifies SetDefault on a secret param
// applies the default string via Value().
func TestSetDefault_Secret(t *testing.T) {
	p := NewSecret("key", "API Key")
	p.def = "sk-test"
	p.SetDefault(map[string]any{})
	got := p.Value()
	want := Value{Kind: TypeSecret, String: "sk-test"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Value() = %+v, want %+v", got, want)
	}
}

// BenchmarkSetDefaultText measures SetDefault() for a text param.
func BenchmarkSetDefaultText(b *testing.B) {
	p := NewText("name", "Name", "default")
	b.ResetTimer()
	for b.Loop() {
		p.SetDefault(map[string]any{})
	}
}

// BenchmarkSetDefaults_Small measures SetDefaults for 4 params
// (the common template size).
func BenchmarkSetDefaults_Small(b *testing.B) {
	ps := []Param{
		NewText("name", "Name", "myapp"),
		NewNumber("port", "Port", 8080, nil, nil),
		NewBool("debug", "Debug", true),
		NewSelect("ed", "Edition", []string{"2021", "2024"}, "2021"),
	}
	b.ResetTimer()
	for b.Loop() {
		SetDefaults(ps, map[string]any{})
	}
}

// TestSetDefaults_AllTypes verifies SetDefaults covers every
// param type in a single call.
func TestSetDefaults_AllTypes(t *testing.T) {
	ps := []Param{
		NewText("a", "A", "txt"),
		NewTextarea("b", "B", "area"),
		NewNumber("c", "C", 42, nil, nil),
		NewSelect("d", "D", []string{"x", "y"}, "y"),
		NewMultiSelect("e", "E", []string{"x", "y"}, []string{"x"}),
		NewBool("f", "F", true),
		NewPath("g", "G", "/tmp"),
		NewSecret("h", "H"),
	}
	ps[7].(*SecretParam).def = "shh"
	SetDefaults(ps, map[string]any{})

	want := map[string]Value{
		"a": {Kind: TypeText, String: "txt"},
		"b": {Kind: TypeTextarea, String: "area"},
		"c": {Kind: TypeNumber, Int: 42},
		"d": {Kind: TypeSelect, String: "y"},
		"e": {Kind: TypeMultiSelect, List: []string{"x"}},
		"f": {Kind: TypeBool, Bool: true},
		"g": {Kind: TypePath, Path: "/tmp"},
		"h": {Kind: TypeSecret, String: "shh"},
	}
	for _, p := range ps {
		w, ok := want[p.Name()]
		if !ok {
			t.Errorf("unexpected param %q", p.Name())
			continue
		}
		if got := p.Value(); !reflect.DeepEqual(got, w) {
			t.Errorf("%s Value() = %+v, want %+v", p.Name(), got, w)
		}
	}
}
