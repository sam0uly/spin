package template

import (
	"fmt"

	"charm.land/huh/v2"

	"github.com/sam0uly/spin/internal/params"
)

// BuildForm constructs a huh.Form from the template's params and
// pre-fills it with any values already supplied.
func (t *Template) BuildForm(values map[string]any) (*huh.Form, error) {
	ps, err := params.Parse(t.SpinToml.Params)
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		if v, ok := values[p.Name()]; ok {
			p.Apply(toParamValue(v))
		}
	}
	return params.Form(ps, values), nil
}

// ResolveForm resolves param values for rendering: defaults first,
// then an interactive form or caller-supplied overrides on top. The
// result is unwrapped to raw Go primitives so text/template output is
// sensible, plus any caller-supplied keys that are not params.
func (t *Template) ResolveForm(values map[string]any, interactive bool) (map[string]any, error) {
	ps, err := params.Parse(t.SpinToml.Params)
	if err != nil {
		return nil, err
	}
	if !interactive {
		params.SetDefaults(ps, values)
	} else {
		if err := params.Run(ps, values); err != nil {
			return nil, err
		}
	}
	// Explicit caller values are applied after defaults so they win.
	for _, p := range ps {
		if v, ok := values[p.Name()]; ok {
			p.Apply(toParamValue(v))
		}
	}
	// huh only validates select options on interactive submit, so
	// check non-interactive paths and --param overrides here too.
	if err := params.ValidateDefaults(ps); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, p := range ps {
		out[p.Name()] = UnwrapValue(p.Value())
	}
	for k, v := range values {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out, nil
}

func toParamValue(v any) params.Value {
	return params.FromAny(v)
}

// UnwrapValue returns the primitive a params.Value holds, since
// text/template wants raw Go types rather than the Value struct.
func UnwrapValue(v params.Value) any {
	if v.Kind != "" {
		switch v.Kind {
		case params.TypeNumber:
			return v.Int
		case params.TypeBool:
			return v.Bool
		case params.TypeMultiSelect:
			return v.List
		case params.TypePath:
			return v.Path
		case params.TypeSecret, params.TypeText, params.TypeTextarea, params.TypeSelect, params.TypeLicense:
			return v.String
		}
	}
	// Fallback for Values without Kind.
	switch {
	case v.List != nil:
		return v.List
	case v.String != "":
		return v.String
	case v.Int != 0:
		return v.Int
	case v.Bool:
		return true
	case v.Path != "":
		return v.Path
	}
	return ""
}

// Hints returns a one-line-per-param summary used by --print-params.
func (t *Template) Hints() []string {
	out := []string{}
	for name, spec := range t.SpinToml.Params {
		out = append(out, fmt.Sprintf("  %-20s %s", name, spec.Type))
	}
	return out
}
