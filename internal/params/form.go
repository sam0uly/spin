package params

import (
	"fmt"
	"slices"

	"charm.land/huh/v2"
)

// PageSize is the number of params grouped per "page" in the huh
// form. huh renders each Group as a separate page (user navigates
// with Next/Prev), so this cap is also the visible pagination
// granularity. 4 keeps the page scannable on a 24-line terminal.
const PageSize = 4

// Form builds a huh.Form from a slice of params. Params are
// grouped PageSize-at-a-time into huh.Groups; each Group becomes
// one form page the user can step through with Next/Prev. A single
// page is rendered when len(ps) <= PageSize.
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

// Run executes the form on the given params. The form populates each
// param's value in place. values are the currently known template
// values, used to render prompts/defaults.
func Run(ps []Param, values map[string]any) error {
	return Form(ps, values).Run()
}

// SetDefaults applies each param's default value to its current value.
// Useful when running non-interactively. values are used to render any
// templated defaults.
func SetDefaults(ps []Param, values map[string]any) {
	for _, p := range ps {
		p.SetDefault(values)
	}
}

// FromAny converts a raw CLI/default value (string, int, bool,
// []string, []any) into a params.Value suitable for Param.Apply. The
// Value's Kind is derived from the Go type; each param's Apply picks
// the field it cares about, so a string maps cleanly onto text,
// select, textarea, secret and path params.
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

// ValidateDefaults checks resolved param values for consistency in the
// non-interactive path, where huh's per-field Validate does not run.
// It enforces that a select param's value is one of its options: a
// templated default (e.g. `default = "{{ .ed }}"`) or a --param value
// that resolves outside the option list would otherwise pass through
// silently and produce a broken scaffold.
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
