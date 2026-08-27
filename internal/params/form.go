package params

import (
	"fmt"
	"slices"

	"charm.land/huh/v2"
)

// PageSize is how many params share one form page in the huh form.
// Four keeps a page scannable on a 24-line terminal.
const PageSize = 4

// Form builds a huh.Form from ps, grouped PageSize params per page.
func Form(ps []Param, values map[string]any) *huh.Form {
	if len(ps) == 0 {
		return huh.NewForm()
	}
	groups := make([]*huh.Group, 0, (len(ps)+PageSize-1)/PageSize)
	for i := 0; i < len(ps); i += PageSize {
		end := min(i+PageSize, len(ps))
		fields := make([]huh.Field, 0, end-i)
		for _, p := range ps[i:end] {
			fields = append(fields, p.HuhField(values))
		}
		groups = append(groups, huh.NewGroup(fields...))
	}
	return huh.NewForm(groups...)
}

// Run executes the form on ps, populating each param's value in
// place.
func Run(ps []Param, values map[string]any) error {
	return Form(ps, values).Run()
}

// SetDefaults applies each param's default value; used when running
// non-interactively.
func SetDefaults(ps []Param, values map[string]any) {
	for _, p := range ps {
		p.SetDefault(values)
	}
}

// FromAny converts a raw CLI or default value into a Value whose Kind
// is derived from the Go type. Each param's Apply picks the field it
// cares about.
func FromAny(v any) Value {
	switch x := v.(type) {
	case string:
		return Value{Kind: TypeText, String: x}
	case int:
		return Value{Kind: TypeNumber, Int: x}
	case bool:
		return Value{Kind: TypeBool, Bool: x}
	case []string:
		return Value{Kind: TypeMultiSelect, List: x}
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return Value{Kind: TypeMultiSelect, List: out}
	}
	return Value{}
}

// ValidateDefaults checks select-style params in the non-interactive
// path, where huh's per-field validation does not run: a value outside
// the option list is an error rather than a broken scaffold.
func ValidateDefaults(ps []Param) error {
	for _, p := range ps {
		var s *SelectParam
		switch x := p.(type) {
		case *SelectParam:
			s = x
		case *LicenseParam:
			s = x.SelectParam
		}
		if s == nil {
			continue
		}
		if s.value != "" && !slices.Contains(s.options, s.value) {
			return fmt.Errorf("%s must be one of %v, got %q", s.name, s.options, s.value)
		}
	}
	return nil
}
