package template

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sam0uly/spin/internal/log"
	"github.com/sam0uly/spin/internal/registry"
	srcspec "github.com/sam0uly/spin/internal/spec"
	"github.com/sam0uly/spin/internal/version"
)

// Loader fetches a template from a local path, git URL, registry
// shorthand, or pinned name, and returns it ready to render.
type Loader struct {
	CacheDir string // clone destination; defaults to ~/.config/spin/templates

	// PromptInvalidPinned is called when a template exists on disk but
	// fails validation. Return true to keep the clone, false to remove
	// it. A nil hook keeps the clone.
	PromptInvalidPinned func(name, localPath string, detectErr error) (bool, error)

	// PromptExistingDest is called when cloneGit finds the destination
	// already populated. A nil hook wipes and re-clones.
	PromptExistingDest func(name, localPath string) (DestAction, error)
}

// defaultInvalidPinnedPrompt keeps the clone; non-interactive runs
// don't delete user data.
func defaultInvalidPinnedPrompt(_, _ string, _ error) (bool, error) {
	return true, nil
}

// defaultExistingDestPrompt wipes and re-clones.
func defaultExistingDestPrompt(_, _ string) (DestAction, error) {
	return DestWipe, nil
}

// NewLoader returns a Loader using cacheDir, or the default cache dir
// when empty.
func NewLoader(cacheDir string) *Loader {
	if cacheDir == "" {
		cacheDir = defaultCacheDir()
	}
	return &Loader{CacheDir: cacheDir}
}

// Load calls LoadContext with a background context.
func (l *Loader) Load(spec string) (*Template, error) {
	return l.LoadContext(context.Background(), spec)
}

// LoadContext fetches a template by spec: a local path, git URL,
// `<alias>/<id>` registry shorthand, or pinned name. The context
// bounds any git clone performed.
func (l *Loader) LoadContext(ctx context.Context, spec string) (*Template, error) {
	var t *Template
	var err error

	if srcspec.IsLocalPath(spec) {
		t, err = Detect(spec)
	} else if srcspec.IsGitURL(spec) {
		t, err = l.cloneGit(ctx, spec)
	} else if registry.IsShorthand(spec) {
		t, err = l.loadShorthand(ctx, spec)
	} else {
		t, err = l.loadPinned(ctx, spec)
		if t == nil && err == nil {
			return nil, fmt.Errorf("%q is not a local path, git URL, or pinned name (run `spin add %s` first to pin a git URL or registry shorthand)", spec, spec)
		}
	}
	if err != nil {
		return nil, err
	}
	if err := l.checkMinSpinVersion(t); err != nil {
		return nil, err
	}
	return t, nil
}

// loadShorthand resolves `<alias>/<id>` via the registry manager and
// routes the resolved source through the normal git/local paths. The
// shorthand itself is kept as Template.Spec so pin prompts can offer
// it back. A resolved source that is already pinned locally reuses
// the cached copy instead of re-cloning.
func (l *Loader) loadShorthand(ctx context.Context, spec string) (*Template, error) {
	mgr := registry.NewManager()
	resolved, err := mgr.ResolveShorthand(ctx, spec)
	if err != nil {
		return nil, err
	}

	// Reuse an existing pin of the same source instead of re-cloning.
	if srcspec.IsGitURL(resolved.Source) {
		client := registry.New()
		if pinned, err := client.ListPinned(ctx); err == nil {
			for _, p := range pinned {
				if p.Source == resolved.Source && p.LocalPath != "" {
					if t, err := Detect(p.LocalPath); err == nil {
						t.Spec = spec
						t.Repo = resolved.Source
						return t, nil
					}
				}
			}
		}
	}

	var tpl *Template
	switch {
	case srcspec.IsLocalPath(resolved.Source):
		tpl, err = Detect(resolved.Source)
	case srcspec.IsGitURL(resolved.Source):
		tpl, err = l.cloneGit(ctx, resolved.Source)
	default:
		return nil, fmt.Errorf("registry: shorthand %q resolved to %q which is neither a local path nor a git URL", spec, resolved.Source)
	}
	if err != nil {
		return nil, err
	}
	if tpl != nil {
		tpl.Spec = spec
		if srcspec.IsGitURL(resolved.Source) {
			tpl.Repo = resolved.Source
		}
	}
	return tpl, nil
}

// loadPinned looks up spec in pinned.json. It returns (nil, nil) when
// the name is not pinned; that is a fall-through, not an error.
func (l *Loader) loadPinned(ctx context.Context, spec string) (*Template, error) {
	client := registry.New()
	pinned, err := client.ListPinned(ctx)
	if err != nil {
		return nil, fmt.Errorf("read pinned: %w", err)
	}
	for _, p := range pinned {
		if p.Name == spec {
			if _, err := os.Stat(p.LocalPath); err != nil {
				return nil, fmt.Errorf("pinned %q missing on disk at %s -- re-run `spin add %s`", p.Name, p.LocalPath, p.Source)
			}
			t, err := Detect(p.LocalPath)
			if err != nil {
				// The clone is malformed; let the user decide whether to
				// keep it for manual repair or drop it.
				prompt := l.PromptInvalidPinned
				if prompt == nil {
					prompt = defaultInvalidPinnedPrompt
				}
				if keep, perr := prompt(p.Name, p.LocalPath, err); perr == nil && !keep {
					if rerr := client.Unpin(ctx, p.Name); rerr == nil {
						if rmErr := os.RemoveAll(p.LocalPath); rmErr != nil {
							log.Debug("failed to remove invalid pinned template", "path", p.LocalPath, "err", rmErr)
						}
					}
					return nil, fmt.Errorf("pinned %q removed (was: %w)", p.Name, err)
				}
				return nil, fmt.Errorf("pinned %q: %w", p.Name, err)
			}
			return t, nil
		}
	}
	return nil, nil
}

// checkMinSpinVersion rejects templates whose spin.toml requires a
// newer spin than the running binary.
func (l *Loader) checkMinSpinVersion(t *Template) error {
	if t.SpinToml == nil || t.SpinToml.MinSpinVersion == "" {
		return nil
	}
	if compareSemver(t.SpinToml.MinSpinVersion, version.Version) > 0 {
		return fmt.Errorf("template %q requires spin >= %s (running %s)", t.Name, t.SpinToml.MinSpinVersion, version.Version)
	}
	return nil
}

func (l *Loader) cloneGit(ctx context.Context, url string) (*Template, error) {
	name := registry.SanitiseRepoName(url)
	dest := filepath.Join(l.CacheDir, name)

	if l.destExists(dest) {
		prompt := l.PromptExistingDest
		if prompt == nil {
			prompt = defaultExistingDestPrompt
		}
		action, aerr := prompt(name, dest)
		if aerr != nil {
			return nil, aerr
		}
		switch action {
		case DestReuse, DestPin:
			tpl, err := l.detectOrPromptInvalid(name, dest)
			if err != nil {
				return nil, err
			}
			tpl.Repo = url
			return tpl, nil
		case DestWipe:
			if err := os.RemoveAll(dest); err != nil {
				return nil, fmt.Errorf("clear cache for %s: %w", dest, err)
			}
		case DestCancel:
			return nil, fmt.Errorf("%q exists at %s; cancelled", name, dest)
		}
	}

	if err := registry.GitClone(ctx, url, dest); err != nil {
		return nil, err
	}
	tpl, err := l.detectOrPromptInvalid(name, dest)
	if err != nil {
		return nil, err
	}
	tpl.Repo = url
	return tpl, nil
}

func (l *Loader) destExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DestAction is the user's choice when a pre-existing clone is found
// at the destination.
type DestAction int

const (
	DestReuse  DestAction = iota // use the existing clone as-is
	DestPin                      // reuse and persist the source for offline use
	DestWipe                     // remove the clone and re-clone
	DestCancel                   // abort without changes
)

// detectOrPromptInvalid runs Detect(dest), routing malformed clones
// through PromptInvalidPinned so both the fresh-clone and reuse paths
// share the handling.
func (l *Loader) detectOrPromptInvalid(name, dest string) (*Template, error) {
	t, err := Detect(dest)
	if err == nil {
		return t, nil
	}
	prompt := l.PromptInvalidPinned
	if prompt == nil {
		prompt = defaultInvalidPinnedPrompt
	}
	keep, perr := prompt(name, dest, err)
	if perr != nil {
		return nil, perr
	}
	if !keep {
		if rmErr := os.RemoveAll(dest); rmErr != nil {
			log.Debug("failed to remove invalid template clone", "path", dest, "err", rmErr)
		}
		return nil, fmt.Errorf("%q at %s removed (was: %w)", name, dest, err)
	}
	return nil, fmt.Errorf("%q at %s: %w", name, dest, err)
}

// Lister returns the basenames of all top-level entries in the cache
// directory.
func (l *Loader) Lister() ([]string, error) {
	entries, err := os.ReadDir(l.CacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out, nil
}

// Clear removes the cached clone named ref (the sanitised repo name).
// Removing an uncached ref is a no-op.
func (l *Loader) Clear(ref string) error {
	dest := filepath.Join(l.CacheDir, ref)
	if _, err := os.Stat(dest); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.RemoveAll(dest)
}

// compareSemver compares dotted numeric versions component by
// component. Missing components count as zero, so "1.0" equals
// "1.0.0". Malformed segments also count as zero.
func compareSemver(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := max(len(as), len(bs))
	for i := range n {
		var ai, bi int
		if i < len(as) {
			ai, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bi, _ = strconv.Atoi(bs[i])
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func defaultCacheDir() string {
	if base, err := os.UserConfigDir(); err == nil && base != "" {
		return filepath.Join(base, "spin", "templates")
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".config", "spin", "templates")
	}
	return "/tmp/spin-templates"
}
