package template

import (
	"strings"
	"testing"

	"samouly.fun/spin/internal/params"
)

// TestSnakeCase verifies the snakeCase helper that converts
// PascalCase/camelCase identifiers to snake_case.
func TestSnakeCase(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"MyProject", "my_project"},
		{"ABC", "abc"},
		{"helloWorld", "hello_world"},
		{"", ""},
		{"already_snake", "already_snake"},
		{"MyXMLParser", "my_xmlparser"},
		{"getHTTPResponse", "get_httpresponse"},
		{"HTMLParser", "htmlparser"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := params.SnakeCase(c.in)
			if got != c.want {
				t.Errorf("snakeCase(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestShellQuote verifies the shellQuote helper that wraps a string
// in single quotes, escaping any embedded single quotes.
func TestShellQuote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "''"},
		{"foo", "'foo'"},
		{"it's", "'it'\"'\"'s'"},
		{"hello world", "'hello world'"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := params.ShellQuote(c.in)
			if got != c.want {
				t.Errorf("shellQuote(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestFuncMapHelpers verifies the key template helpers registered
// by funcMap(), ensuring they produce the expected input→output.
func TestFuncMapHelpers(t *testing.T) {
	fm := params.FuncMap()

	t.Run("upper", func(t *testing.T) {
		fn, ok := fm["upper"]
		if !ok {
			t.Fatal("upper not in funcMap")
		}
		got := fn.(func(string) string)("hello")
		if got != strings.ToUpper("hello") {
			t.Errorf("upper(%q) = %q, want %q", "hello", got, strings.ToUpper("hello"))
		}
	})

	t.Run("lower", func(t *testing.T) {
		fn, ok := fm["lower"]
		if !ok {
			t.Fatal("lower not in funcMap")
		}
		got := fn.(func(string) string)("WORLD")
		if got != strings.ToLower("WORLD") {
			t.Errorf("lower(%q) = %q, want %q", "WORLD", got, strings.ToLower("WORLD"))
		}
	})

	t.Run("trim", func(t *testing.T) {
		fn, ok := fm["trim"]
		if !ok {
			t.Fatal("trim not in funcMap")
		}
		got := fn.(func(string) string)("  hello  ")
		if got != strings.TrimSpace("  hello  ") {
			t.Errorf("trim(%q) = %q, want %q", "  hello  ", got, strings.TrimSpace("  hello  "))
		}
	})

	t.Run("contains", func(t *testing.T) {
		fn, ok := fm["contains"]
		if !ok {
			t.Fatal("contains not in funcMap")
		}
		got := fn.(func(string, string) bool)("hello world", "world")
		if !got {
			t.Errorf("contains(%q, %q) = false, want true", "hello world", "world")
		}
	})

	t.Run("default_nil", func(t *testing.T) {
		fn, ok := fm["default"]
		if !ok {
			t.Fatal("default not in funcMap")
		}
		got := fn.(func(any, any) any)("fallback", nil)
		if got != "fallback" {
			t.Errorf("default(%q, nil) = %v, want %q", "fallback", got, "fallback")
		}
	})

	t.Run("default_empty", func(t *testing.T) {
		fn, ok := fm["default"]
		if !ok {
			t.Fatal("default not in funcMap")
		}
		got := fn.(func(any, any) any)("fallback", "")
		if got != "fallback" {
			t.Errorf("default(%q, %q) = %v, want %q", "fallback", "", got, "fallback")
		}
	})

	t.Run("default_nonempty", func(t *testing.T) {
		fn, ok := fm["default"]
		if !ok {
			t.Fatal("default not in funcMap")
		}
		got := fn.(func(any, any) any)("fallback", "actual")
		if got != "actual" {
			t.Errorf("default(%q, %q) = %v, want %q", "fallback", "actual", got, "actual")
		}
	})

	t.Run("has_present", func(t *testing.T) {
		fn, ok := fm["has"]
		if !ok {
			t.Fatal("has not in funcMap")
		}
		got := fn.(func([]string, string) bool)([]string{"a", "b", "c"}, "b")
		if !got {
			t.Error("has([a b c], b) = false, want true")
		}
	})

	t.Run("has_absent", func(t *testing.T) {
		fn, ok := fm["has"]
		if !ok {
			t.Fatal("has not in funcMap")
		}
		got := fn.(func([]string, string) bool)([]string{"a", "b", "c"}, "d")
		if got {
			t.Error("has([a b c], d) = true, want false")
		}
	})

	t.Run("not_has_present", func(t *testing.T) {
		fn, ok := fm["not_has"]
		if !ok {
			t.Fatal("not_has not in funcMap")
		}
		got := fn.(func([]string, string) bool)([]string{"a", "b"}, "a")
		if got {
			t.Error("not_has([a b], a) = true, want false")
		}
	})

	t.Run("not_has_absent", func(t *testing.T) {
		fn, ok := fm["not_has"]
		if !ok {
			t.Fatal("not_has not in funcMap")
		}
		got := fn.(func([]string, string) bool)([]string{"a", "b"}, "z")
		if !got {
			t.Error("not_has([a b], z) = false, want true")
		}
	})

	t.Run("one_of_match", func(t *testing.T) {
		fn, ok := fm["one_of"]
		if !ok {
			t.Fatal("one_of not in funcMap")
		}
		got := fn.(func(string, ...string) bool)("b", "a", "b", "c")
		if !got {
			t.Error("one_of(b, a b c) = false, want true")
		}
	})

	t.Run("one_of_no_match", func(t *testing.T) {
		fn, ok := fm["one_of"]
		if !ok {
			t.Fatal("one_of not in funcMap")
		}
		got := fn.(func(string, ...string) bool)("z", "a", "b", "c")
		if got {
			t.Error("one_of(z, a b c) = true, want false")
		}
	})
}
