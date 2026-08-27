package template

import (
	"fmt"

	"github.com/BurntSushi/toml"

	"github.com/sam0uly/spin/internal/licenses"
	"github.com/sam0uly/spin/internal/params"
)

// rawSpinToml is the intermediate decode target for spin.toml. Params
// stays a map of any so the shorthand `name = "default"` and the full
// `name = { type = ... }` forms share one field; parseTOML converts
// them to params.Spec afterwards.
type rawSpinToml struct {
	Name           string         `toml:"name"`
	Description    string         `toml:"description"`
	Type           string         `toml:"type"`
	Language       string         `toml:"language"`
	Author         rawAuthor      `toml:"author"`
	License        string         `toml:"license"`
	Repository     string         `toml:"repository"`
	MinSpinVersion string         `toml:"min_spin_version"`
	Exclude        []string       `toml:"exclude"`
	Include        []IncludeRule  `toml:"include"`
	Params         map[string]any `toml:"params"`
	Pre            []PreStep      `toml:"pre"`
	Post           []PostStep     `toml:"post"`
	Tags           []string       `toml:"tags"`
}

type rawAuthor struct {
	Name  string `toml:"name"`
	Email string `toml:"email"`
	URL   string `toml:"url"`
}

// parseTOML decodes a spin.toml document into st, converting each
// [params] entry (shorthand string or inline table) into a
// params.Spec.
func parseTOML(b []byte, st *SpinToml) error {
	var raw rawSpinToml
	if err := toml.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("spin.toml: %w", err)
	}

	st.Name = raw.Name
	st.Description = raw.Description
	st.Type = raw.Type
	st.Language = raw.Language
	st.Author = Author(raw.Author)
	st.License = raw.License
	st.Repository = raw.Repository
	st.MinSpinVersion = raw.MinSpinVersion
	st.Exclude = raw.Exclude
	st.Include = raw.Include
	st.Post = raw.Post
	st.Pre = raw.Pre
	st.Tags = raw.Tags

	for k, v := range raw.Params {
		spec, err := coerceParamValue(v)
		if err != nil {
			return fmt.Errorf("param %q: %w", k, err)
		}
		st.Params[k] = spec
	}
	return nil
}

// coerceParamValue converts one [params] entry into a params.Spec.
// A string is shorthand for a text param with that default; an inline
// table is the full form.
func coerceParamValue(v any) (params.Spec, error) {
	switch x := v.(type) {
	case string:
		return params.Spec{Type: params.TypeText, Default: x}, nil
	case map[string]any:
		return specFromMap(x), nil
	case nil:
		return params.Spec{}, nil
	default:
		return params.Spec{}, fmt.Errorf("unsupported value type %T (want string or inline table)", v)
	}
}

func specFromMap(m map[string]any) params.Spec {
	spec := params.Spec{}
	if s, ok := m["type"].(string); ok {
		spec.Type = params.Type(s)
	}
	if s, ok := m["prompt"].(string); ok {
		spec.Prompt = s
	}
	if d, ok := m["default"]; ok {
		spec.Default = d
	}
	if n, ok := asInt64(m["min"]); ok {
		v := int(n)
		spec.Min = &v
	}
	if n, ok := asInt64(m["max"]); ok {
		v := int(n)
		spec.Max = &v
	}
	if opts, ok := m["options"].([]any); ok {
		for _, o := range opts {
			if s, ok := o.(string); ok {
				spec.Options = append(spec.Options, s)
			}
		}
	}
	// License params default to the built-in option set.
	if spec.Type == params.TypeLicense && len(spec.Options) == 0 {
		spec.Options = licenses.Options()
	}
	return spec
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	}
	return 0, false
}
