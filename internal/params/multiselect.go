package params

import (
	"fmt"

	"charm.land/huh/v2"
)

// MultiSelectParam is a multi-choice prompt over a fixed option list.
type MultiSelectParam struct {
	name    string
	prompt  string
	options []string
	def     []string
	value   []string
}

func NewMultiSelect(name, prompt string, options, def []string) *MultiSelectParam {
	return &MultiSelectParam{name: name, prompt: prompt, options: options, def: def, value: []string{}}
}

func (p *MultiSelectParam) Name() string   { return p.name }
func (p *MultiSelectParam) Type() Type     { return TypeMultiSelect }
func (p *MultiSelectParam) Prompt() string { return p.prompt }
func (p *MultiSelectParam) Default() any   { return p.def }
func (p *MultiSelectParam) SetDefault(values map[string]any) {
	p.Apply(Value{Kind: TypeMultiSelect, List: p.def})
}
func (p *MultiSelectParam) Apply(v Value) {
	if v.List == nil {
		p.value = []string{}
	} else {
		p.value = v.List
	}
}
func (p *MultiSelectParam) Value() Value { return Value{Kind: TypeMultiSelect, List: p.value} }

func (p *MultiSelectParam) HuhField(values map[string]any) huh.Field {
	opts := make([]huh.Option[string], 0, len(p.options))
	for _, o := range p.options {
		opt := huh.NewOption(o, o)
		for _, d := range p.def {
			if o == d {
				opt = opt.Selected(true)
			}
		}
		opts = append(opts, opt)
	}
	return huh.NewMultiSelect[string]().
		Key(p.name).
		Title(orPrompt(p.name, renderStr(p.prompt, values))).
		Options(opts...).
		Value(&p.value)
}

func (p *MultiSelectParam) String() string {
	return fmt.Sprintf("%s (multiselect, default %v, options %v)", p.name, p.def, p.options)
}
