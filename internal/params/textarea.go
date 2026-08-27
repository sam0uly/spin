package params

import (
	"fmt"

	"charm.land/huh/v2"
)

// TextareaParam is a multi-line text input with no character limit.
type TextareaParam struct {
	name   string
	prompt string
	def    string
	value  string
}

func NewTextarea(name, prompt, def string) *TextareaParam {
	return &TextareaParam{name: name, prompt: prompt, def: def}
}

func (p *TextareaParam) Name() string   { return p.name }
func (p *TextareaParam) Type() Type     { return TypeTextarea }
func (p *TextareaParam) Prompt() string { return p.prompt }
func (p *TextareaParam) Default() any   { return p.def }
func (p *TextareaParam) SetDefault(values map[string]any) {
	p.Apply(Value{Kind: TypeTextarea, String: renderStr(p.def, values)})
}
func (p *TextareaParam) Apply(v Value) { p.value = v.String }
func (p *TextareaParam) Value() Value  { return Value{Kind: TypeTextarea, String: p.value} }
func (p *TextareaParam) HuhField(values map[string]any) huh.Field {
	f := huh.NewText().
		Key(p.name).
		Title(orPrompt(p.name, renderStr(p.prompt, values))).
		CharLimit(0).
		Value(&p.value)
	if r := renderStr(p.def, values); r != "" {
		f = f.Placeholder(r)
	}
	return f
}

func (p *TextareaParam) String() string {
	return fmt.Sprintf("%s (textarea, default %q)", p.name, p.def)
}
