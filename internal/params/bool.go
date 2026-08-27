package params

import (
	"fmt"

	"charm.land/huh/v2"
)

// BoolParam is a yes/no confirm prompt.
type BoolParam struct {
	name   string
	prompt string
	def    bool
	value  bool
}

func NewBool(name, prompt string, def bool) *BoolParam {
	return &BoolParam{name: name, prompt: prompt, def: def}
}

func (p *BoolParam) Name() string                     { return p.name }
func (p *BoolParam) Type() Type                       { return TypeBool }
func (p *BoolParam) Prompt() string                   { return p.prompt }
func (p *BoolParam) Default() any                     { return p.def }
func (p *BoolParam) SetDefault(values map[string]any) { p.Apply(Value{Kind: TypeBool, Bool: p.def}) }
func (p *BoolParam) Apply(v Value)                    { p.value = v.Bool }
func (p *BoolParam) Value() Value                     { return Value{Kind: TypeBool, Bool: p.value} }
func (p *BoolParam) HuhField(values map[string]any) huh.Field {
	return huh.NewConfirm().
		Key(p.name).
		Title(orPrompt(p.name, renderStr(p.prompt, values))).
		Affirmative("Yes").
		Negative("No").
		Value(&p.value)
}

func (p *BoolParam) String() string {
	return fmt.Sprintf("%s (bool, default %t)", p.name, p.def)
}
