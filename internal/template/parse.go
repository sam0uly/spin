package template

import (
	"fmt"

	"github.com/BurntSushi/toml"

	"github.com/sam0uly/spin/internal/licenses"
	"github.com/sam0uly/spin/internal/params"
)

// rawSpinToml is the intermediate shape BurntSushi/toml decodes into.
// We hold [params] as a map of any so the shorthand `name = "default"`
// (string) and the full form `name = { type = "...", ... }` (inline
// table) can both land in the same map; the post-pass below turns the
// any into a concrete params.Spec.
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

// parseTOML decodes a spin.toml document. It uses BurntSushi/toml
// (the de-facto Go TOML library, originally proposed for stdlib) for
// the heavy lifting, then walks the raw params map to convert
// shorthand entries (`name = "default"`) into a text-typed params.Spec
// and inline-table entries (`name = { type = "select", ... }`) into
// a fully populated Spec.
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

// coerceParamValue turns one entry of [params] into a params.Spec.
// The input is the any BurntSushi/toml produced: a string for the
// shorthand, a map[string]any for the inline-table form.
func coerceParamValue(v any) (params.Spec, error) {
	switch x := v.(type) {
	case string:
		// shorthand: name = "default" ⇒ { type = "text", default = "default" }
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
	// A license param with no options gets the built-in set, so
	// `--param license=MIT` validates against the curated list and
	// the interactive form has something to offer.
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
