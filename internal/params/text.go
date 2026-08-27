package params

import (
	"fmt"

	"charm.land/huh/v2"
)

// TextParam is a single-line text input with an optional default shown
// as placeholder.
type TextParam struct {
	name   string
	prompt string
	def    string
	value  string
}

func NewText(name, prompt, def string) *TextParam {
	return &TextParam{name: name, prompt: prompt, def: def}
}

func (p *TextParam) Name() string   { return p.name }
func (p *TextParam) Type() Type     { return TypeText }
func (p *TextParam) Prompt() string { return p.prompt }
func (p *TextParam) Default() any   { return p.def }
func (p *TextParam) SetDefault(values map[string]any) {
	p.Apply(Value{Kind: TypeText, String: renderStr(p.def, values)})
}
func (p *TextParam) Apply(v Value) { p.value = v.String }
func (p *TextParam) Value() Value  { return Value{Kind: TypeText, String: p.value} }
func (p *TextParam) HuhField(values map[string]any) huh.Field {
	f := huh.NewInput().
		Key(p.name).
		Title(orPrompt(p.name, renderStr(p.prompt, values))).
		Value(&p.value)
	if r := renderStr(p.def, values); r != "" {
		f = f.Placeholder(r)
	}
	return f
}

func (p *TextParam) String() string {
	return fmt.Sprintf("%s (text, default %q)", p.name, p.def)
}
