package template

import (
	"fmt"

	"charm.land/huh/v2"

	"github.com/sam0uly/spin/internal/params"
)

// BuildForm constructs a huh.Form from the template's spin.toml params.
// The user fills the form; the resolved values are written back into
// the supplied map.
func (t *Template) BuildForm(values map[string]any) (*huh.Form, error) {
	ps, err := params.Parse(t.SpinToml.Params)
	if err != nil {
		return nil, err
	}
	// Pre-fill with the supplied values, so the user sees existing defaults
	// (e.g. when re-running with --no-interactive).
	for _, p := range ps {
		if v, ok := values[p.Name()]; ok {
			p.Apply(toParamValue(v))
		}
	}
	form := params.Form(ps, values)
	// After Run, walk the params and copy their Values into values.
	// We attach a callback by using huh's Key() -- but huh doesn't have
	// a generic "post-run" hook. So we expose a separate helper.
	return form, nil
}

// ResolveForm runs the form (or applies defaults in non-interactive
// mode) and returns the resolved values ready for Render().
//
// Returned values are unwrapped to raw Go primitives (string, int,
// bool, []string) so text/template rendering produces sensible
// output (e.g. `{{.project_name}}` interpolates as the name, not
// the params.Value struct dump).
//
// Order of operations is significant: defaults are applied first,
// THEN any caller-supplied values are layered on top. This ensures
// explicit values from the CLI or pre-apply map win over the
// template's own defaults.
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
	// Apply caller-supplied values AFTER defaults so explicit
	// overrides win.
	for _, p := range ps {
		if v, ok := values[p.Name()]; ok {
			p.Apply(toParamValue(v))
		}
	}
	// huh validates select values on submit in interactive mode, but
	// the non-interactive path (and --param overrides) skip that, so
	// reject a select value outside its options here.
	if err := params.ValidateDefaults(ps); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, p := range ps {
		// Unwrap params.Value to its underlying primitive so
		// text/template sees {{.project_name}} as a string, not
		// the Value struct dump.
		out[p.Name()] = UnwrapValue(p.Value())
	}
	// also copy through any caller-supplied keys that aren't params
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

// UnwrapValue returns the underlying primitive held by a
// params.Value. The text/template engine wants raw Go types
// (string, int, bool, []string), not the multi-field struct.
// Exported because post_hook.go also needs it.
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
	// Fallback for Values without Kind (legacy callers).
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

// Hints returns a one-line-per-param summary, used by
// `spin new <template> --print-params` and the template README.
func (t *Template) Hints() []string {
	out := []string{}
	for name, spec := range t.SpinToml.Params {
		out = append(out, fmt.Sprintf("  %-20s %s", name, spec.Type))
	}
	return out
}
