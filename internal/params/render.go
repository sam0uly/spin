package params

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// FuncMap returns the template helpers available to file rendering
// and to param prompts/defaults alike, so authors can use the same
// functions in both places.
func FuncMap() template.FuncMap {
	titleCaser := cases.Title(language.English)
	return template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": titleCaser.String,
		"trim":  strings.TrimSpace,
		"join":  strings.Join,
		"default": func(d, v any) any {
			if v == nil || v == "" {
				return d
			}
			return v
		},
		"snake_case": SnakeCase,
		"kebab": func(s string) string {
			return strings.ReplaceAll(SnakeCase(s), "_", "-")
		},
		"quote": ShellQuote,
		// now formats the current time; an empty layout means RFC3339.
		"now": func(layout string) string {
			if layout == "" {
				layout = time.RFC3339
			}
			return time.Now().UTC().Format(layout)
		},
		"contains": strings.Contains,
		// has reports list membership; not_has and one_of are variants.
		"has": slices.Contains[[]string, string],
		"not_has": func(list []string, item string) bool {
			return !slices.Contains(list, item)
		},
		"one_of": func(v string, items ...string) bool {
			return slices.Contains(items, v)
		},
	}
}

var nonWordSplitter = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// SnakeCase converts a PascalCase/camelCase identifier to snake_case.
// SnakeCase converts a PascalCase or camelCase identifier to
// snake_case, splitting on case and non-alphanumeric boundaries.
func SnakeCase(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && isUpper(r) && !isUpper(runes[i-1]) {
			b.WriteByte('_')
		}
		if i > 0 && isUpper(r) && i+1 < len(runes) && isLower(runes[i+1]) && isLower(runes[i-1]) {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(nonWordSplitter.ReplaceAllString(b.String(), "_"))
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }

// ShellQuote wraps s in single quotes, escaping any embedded single
// quotes with the standard "'='"'"' trick.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// renderStr renders s as a Go template against values. Strings with
// no directives, or that fail to parse or execute, are returned
// unchanged so malformed prompts never break scaffolding.
func renderStr(s string, values map[string]any) string {
	if s == "" || !strings.Contains(s, "{{") {
		return s
	}
	t, err := template.New("param").Funcs(FuncMap()).Parse(s)
	if err != nil {
		return s
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, values); err != nil {
		return s
	}
	return buf.String()
}
