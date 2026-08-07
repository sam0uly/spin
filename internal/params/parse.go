package params

import (
	"fmt"
	"sort"
)

// SpecMap is a raw spin.toml `params` block: param name → Spec.
type SpecMap map[string]Spec

// Parse turns a SpecMap into a list of typed Param instances, sorted
// alphabetically for consistent display order.
func Parse(specs SpecMap) ([]Param, error) {
	out := make([]Param, 0, len(specs))
	for name, s := range specs {
		p, err := ParseOne(name, s)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	return out, nil
}

// ParseOne builds a single Param from a name and Spec.
func ParseOne(name string, s Spec) (Param, error) {
	switch s.Type {
	case TypeText, "":
		return NewText(name, s.Prompt, asString(s.Default)), nil
	case TypeTextarea:
		return NewTextarea(name, s.Prompt, asString(s.Default)), nil
	case TypeNumber:
		return NewNumber(name, s.Prompt, asInt(s.Default), s.Min, s.Max), nil
	case TypeSelect:
		return NewSelect(name, s.Prompt, s.Options, asString(s.Default)), nil
	case TypeLicense:
		return NewLicense(name, s.Prompt, s.Options, asString(s.Default)), nil
	case TypeMultiSelect:
		return NewMultiSelect(name, s.Prompt, s.Options, asStringSlice(s.Default)), nil
	case TypeBool:
		return NewBool(name, s.Prompt, asBool(s.Default)), nil
	case TypePath:
		return NewPath(name, s.Prompt, asString(s.Default)), nil
	case TypeSecret:
		return NewSecret(name, s.Prompt), nil
	default:
		return nil, ErrUnknownType{Name: name, Type: s.Type}
	}
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func asBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func asStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, x := range s {
			if str, ok := x.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}
