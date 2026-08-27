package params

import (
	"fmt"
	"slices"

	"charm.land/huh/v2"
)

// SelectParam is a single-choice prompt over a fixed option list.
type SelectParam struct {
	name    string
	prompt  string
	options []string
	def     string
	value   string
}

func NewSelect(name, prompt string, options []string, def string) *SelectParam {
	return &SelectParam{name: name, prompt: prompt, options: options, def: def}
}

func (p *SelectParam) Name() string   { return p.name }
func (p *SelectParam) Type() Type     { return TypeSelect }
func (p *SelectParam) Prompt() string { return p.prompt }
func (p *SelectParam) Default() any   { return p.def }
func (p *SelectParam) SetDefault(values map[string]any) {
	p.Apply(Value{Kind: TypeSelect, String: renderStr(p.def, values)})
}
func (p *SelectParam) Apply(v Value) { p.value = v.String }
func (p *SelectParam) Value() Value  { return Value{Kind: TypeSelect, String: p.value} }
func (p *SelectParam) HuhField(values map[string]any) huh.Field {
	opts := make([]huh.Option[string], 0, len(p.options))
	for _, o := range p.options {
		opts = append(opts, huh.NewOption(o, o))
	}
	f := huh.NewSelect[string]().
		Key(p.name).
		Title(orPrompt(p.name, renderStr(p.prompt, values))).
		Options(opts...).
		Value(&p.value)
	if p.def != "" {
		f = f.Validate(func(s string) error {
			if slices.Contains(p.options, s) {
				return nil
			}
			return fmt.Errorf("not in options: %q", s)
		})
	}
	return f
}

func (p *SelectParam) String() string {
	return fmt.Sprintf("%s (select, default %q, options %v)", p.name, p.def, p.options)
}
