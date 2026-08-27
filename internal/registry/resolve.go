package registry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	srcspec "github.com/sam0uly/spin/internal/spec"
)

// ErrUnresolved is returned when a `<alias>/<id>` shorthand cannot
// be matched to a registered template.
var ErrUnresolved = errors.New("shorthand unresolved")

// AliasNotRegisteredError is returned by ResolveShorthand when the
// alias in an `<alias>/<id>` spec has no registered registry. It
// carries the alias so callers can add their own hints.
type AliasNotRegisteredError struct {
	Alias string
}

func (e AliasNotRegisteredError) Error() string {
	return fmt.Sprintf("alias %q not registered", e.Alias)
}

// SplitAliasID splits a `<alias>/<id>` shorthand into its parts.
func SplitAliasID(spec string) (alias, id string) {
	i := strings.IndexByte(spec, '/')
	return spec[:i], spec[i+1:]
}

// splitAliasID is the unexported alias for internal use.
func splitAliasID(spec string) (alias, id string) {
	return SplitAliasID(spec)
}

// Resolved is the output of ResolveShorthand: the template's source,
// its kind, and the alias/id parts the user typed.
type Resolved struct {
	Alias  string
	ID     string
	Source string
	Kind   RegistryKind
}

// IsShorthand reports whether spec looks like `<alias>/<id>`. URLs
// and filesystem paths do not match.
func IsShorthand(spec string) bool {
	return srcspec.IsShorthand(spec)
}

// ResolveShorthand resolves `<alias>/<id>` against the registered
// registries. It follows one level of shorthand-to-shorthand
// indirection; longer chains are rejected as cycles. Returns
// ErrUnresolved or AliasNotRegisteredError on failure.
func (m Manager) ResolveShorthand(ctx context.Context, spec string) (Resolved, error) {
	if err := ctx.Err(); err != nil {
		return Resolved{}, err
	}
	return m.resolveShorthandDepth(ctx, spec, 0)
}

// resolveShorthandDepth is the recursive helper behind ResolveShorthand.
// depth tracks shorthand chain length to bound cycles.
func (m Manager) resolveShorthandDepth(ctx context.Context, spec string, depth int) (Resolved, error) {
	if depth > 1 {
		return Resolved{}, fmt.Errorf("shorthand chain too deep (cycle?): %s", spec)
	}
	if !IsShorthand(spec) {
		return Resolved{}, fmt.Errorf("shorthand %q is not an <alias>/<id>", spec)
	}
	alias, id := splitAliasID(spec)
	reg, ok := m.Get(ctx, alias)
	if !ok {
		return Resolved{}, AliasNotRegisteredError{Alias: alias}
	}
	tplPath := filepath.Join(reg.Path, "templates", id+".toml")
	if _, err := os.Stat(tplPath); err != nil {
		return Resolved{}, fmt.Errorf("template %q not in registry %q", id, alias)
	}
	var tpl TemplateMetadata
	if _, err := toml.DecodeFile(tplPath, &tpl); err != nil {
		return Resolved{}, fmt.Errorf("parse %s: %w", tplPath, err)
	}
	if tpl.Source == "" {
		return Resolved{}, fmt.Errorf("template %q has empty source", id)
	}
	// If the source is itself a shorthand, follow the chain.
	if IsShorthand(tpl.Source) {
		return m.resolveShorthandDepth(ctx, tpl.Source, depth+1)
	}
	return Resolved{
		Alias:  alias,
		ID:     id,
		Source: tpl.Source,
		Kind:   reg.Kind,
	}, nil
}
