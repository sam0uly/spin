package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/sam0uly/spin/internal/log"
	srcspec "github.com/sam0uly/spin/internal/spec"
)

// ErrAliasInvalid is returned when an alias fails ValidateAlias.
var ErrAliasInvalid = errors.New("invalid alias")

// ErrAliasExists is returned by Manager.Add when the alias is already
// registered and force was not set.
var ErrAliasExists = errors.New("alias already registered")

// ErrRegistryMissing is returned when the named alias is not in
// registries.json.
var ErrRegistryMissing = errors.New("alias not registered")

// Manager owns the registry catalogue (registries.json) and the
// on-disk clones under CacheDir/registries/<alias>/.
type Manager struct {
	CacheDir string // defaults to ~/.config/spin
}

// NewManager returns a Manager rooted at the default config dir. Use
// SetCacheDir for tests to redirect to a temp dir.
func NewManager() *Manager {
	return &Manager{CacheDir: defaultConfigDir()}
}

// SetCacheDir returns a copy of m with a different CacheDir. Keeps
// the Manager value cheap to copy.
func (m Manager) SetCacheDir(dir string) Manager {
	m.CacheDir = dir
	return m
}

func (m Manager) RegistriesPath() string {
	return filepath.Join(m.CacheDir, "registries.json")
}

func (m Manager) RegistriesDir() string {
	return filepath.Join(m.CacheDir, "registries")
}

// ValidateAlias reports whether alias is safe to use as a directory
// name and CLI argument: no path traversal, control characters,
// whitespace, or ':' and NUL.
func ValidateAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("%w: empty", ErrAliasInvalid)
	}
	if len(alias) > 128 {
		return fmt.Errorf("%w: longer than 128 chars", ErrAliasInvalid)
	}
	if alias[0] == '-' {
		return fmt.Errorf("%w: cannot start with '-'", ErrAliasInvalid)
	}
	if strings.Contains(alias, "/") || strings.Contains(alias, "\\") {
		return fmt.Errorf("%w: contains '/ or '\\'", ErrAliasInvalid)
	}
	if strings.Contains(alias, "..") {
		return fmt.Errorf("%w: contains '..'", ErrAliasInvalid)
	}
	if strings.ContainsAny(alias, ":\x00 \t\n\r") {
		return fmt.Errorf("%w: contains whitespace, ':', or NUL", ErrAliasInvalid)
	}
	return nil
}

// Load reads registries.json. A missing file yields an empty config,
// not an error; a corrupt file returns a wrapped error so the CLI
// can suggest deleting it.
func (m Manager) Load(ctx context.Context) (RegistriesConfig, error) {
	if err := ctx.Err(); err != nil {
		return RegistriesConfig{}, err
	}
	var out RegistriesConfig
	b, err := os.ReadFile(m.RegistriesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	if len(b) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("registries.json: %w", err)
	}
	return out, nil
}

func (m Manager) Get(ctx context.Context, alias string) (Registry, bool) {
	if err := ctx.Err(); err != nil {
		return Registry{}, false
	}
	cfg, err := m.Load(ctx)
	if err != nil {
		return Registry{}, false
	}
	for _, r := range cfg.Registries {
		if r.Alias == alias {
			return r, true
		}
	}
	return Registry{}, false
}

func (m Manager) List(ctx context.Context) ([]Registry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg, err := m.Load(ctx)
	if err != nil {
		return nil, err
	}
	return cfg.Registries, nil
}

// Add registers alias pointing at source. Source may be a local path
// or a git URL. force=false refuses to overwrite an existing alias;
// force=true wipes the existing cache before re-cloning/re-linking.
//
// Returns ErrAliasInvalid / ErrAliasExists on validation failure. On
// success returns the freshly-added Registry record (with AddedAt and
// Path set).
func (m Manager) Add(ctx context.Context, alias, source string, force bool) (Registry, error) {
	if err := ValidateAlias(alias); err != nil {
		return Registry{}, err
	}
	if source == "" {
		return Registry{}, fmt.Errorf("empty source")
	}

	cfg, err := m.Load(ctx)
	if err != nil {
		return Registry{}, err
	}
	for _, r := range cfg.Registries {
		if r.Alias == alias && !force {
			return Registry{}, fmt.Errorf("%w: %q (use --force to replace)", ErrAliasExists, alias)
		}
	}

	// Resolve the source kind. Symlink for local paths, shallow
	// clone for git URLs. The clone goes to a sibling temp dir first
	// so a failed clone (no registry.toml, network error) leaves no
	// half-state under registries/<alias>/.
	dest := filepath.Join(m.RegistriesDir(), alias)
	if err := os.MkdirAll(m.RegistriesDir(), 0o755); err != nil {
		return Registry{}, fmt.Errorf("create cache directory: %w", err)
	}

	// Drop any existing cache slot first (--force).
	if force {
		if err := os.RemoveAll(dest); err != nil {
			log.Debug("failed to remove existing registry cache", "path", dest, "err", err)
		}
	}

	kind, err := m.cloneOrLink(ctx, alias, source, dest)
	if err != nil {
		return Registry{}, err
	}

	// The destination must contain registry.toml and templates/, else
	// the source was not a registry: roll back and fail.
	if _, err := os.Stat(filepath.Join(dest, "registry.toml")); err != nil {
		if rmErr := os.RemoveAll(dest); rmErr != nil {
			log.Debug("failed to clean up invalid registry cache", "path", dest, "err", rmErr)
		}
		return Registry{}, fmt.Errorf("source %s is not a registry: missing registry.toml", source)
	}
	if _, err := os.Stat(filepath.Join(dest, "templates")); err != nil {
		if rmErr := os.RemoveAll(dest); rmErr != nil {
			log.Debug("failed to clean up invalid registry cache", "path", dest, "err", rmErr)
		}
		return Registry{}, fmt.Errorf("source %s is not a registry: missing templates/ directory", source)
	}

	reg := Registry{
		Alias:   alias,
		Source:  source,
		Kind:    kind,
		Path:    dest,
		AddedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := m.upsert(ctx, reg); err != nil {
		if rmErr := os.RemoveAll(dest); rmErr != nil {
			log.Debug("failed to clean up registry cache after upsert failure", "path", dest, "err", rmErr)
		}
		return Registry{}, err
	}
	return reg, nil
}

// cloneOrLink symlinks local sources and shallow-clones git sources.
// Git clones land in a sibling temp dir first and are renamed into
// place, so a failed clone leaves nothing behind.
func (m Manager) cloneOrLink(ctx context.Context, alias, source, dest string) (RegistryKind, error) {
	if srcspec.IsLocalPath(source) {
		src, err := expandHome(source)
		if err != nil {
			return "", fmt.Errorf("resolve source: %w", err)
		}
		src, err = filepath.Abs(src)
		if err != nil {
			return "", fmt.Errorf("resolve source: %w", err)
		}
		info, err := os.Stat(src)
		if err != nil {
			return "", fmt.Errorf("source not found: %s", src)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("source %s is not a directory", src)
		}
		if err := os.Symlink(src, dest); err != nil {
			if copyErr := copyDir(src, dest); copyErr != nil {
				return "", fmt.Errorf("link/copy source %s failed: symlink %v, copy %w", src, err, copyErr)
			}
		}
		return KindLocal, nil
	}

	// Git source: clone to a sibling temp dir, validate registry.toml
	// is present, then atomic-rename into place.
	parent := filepath.Dir(dest)
	tmp, err := os.MkdirTemp(parent, alias+".clone-")
	if err != nil {
		return "", fmt.Errorf("create temp directory: %w", err)
	}
	defer func() {
		if _, err := os.Stat(tmp); err == nil {
			if rmErr := os.RemoveAll(tmp); rmErr != nil {
				log.Debug("failed to clean up registry clone temp dir", "path", tmp, "err", rmErr)
			}
		}
	}()

	if err := GitClone(ctx, source, tmp); err != nil {
		return "", fmt.Errorf("cannot clone %s: %w", source, err)
	}

	if err := os.Rename(tmp, dest); err != nil {
		return "", fmt.Errorf("install cloned registry: %w", err)
	}
	return KindGit, nil
}

func (m Manager) upsert(ctx context.Context, reg Registry) error {
	cfg, err := m.Load(ctx)
	if err != nil {
		return err
	}
	found := false
	for i, r := range cfg.Registries {
		if r.Alias == reg.Alias {
			cfg.Registries[i] = reg
			found = true
			break
		}
	}
	if !found {
		cfg.Registries = append(cfg.Registries, reg)
	}
	return m.writeRegistries(cfg)
}

// Refresh fetches and resets a git registry to its upstream HEAD.
// Local registries are a no-op with changed=false. LastUpdated is
// stamped only when HEAD moved. Returns ErrRegistryMissing for an
// unregistered alias.
func (m Manager) Refresh(ctx context.Context, alias string) (Registry, bool, error) {
	cfg, err := m.Load(ctx)
	if err != nil {
		return Registry{}, false, err
	}
	for i, r := range cfg.Registries {
		if r.Alias != alias {
			continue
		}
		if r.Kind == KindLocal {
			return r, false, nil
		}
		before, _ := gitHeadSHA(r.Path)
		if err := GitFetch(ctx, r.Path); err != nil {
			return r, false, err
		}
		if err := GitReset(ctx, r.Path); err != nil {
			return r, false, err
		}
		after, _ := gitHeadSHA(r.Path)
		changed := before != after
		if changed {
			r.LastUpdated = time.Now().UTC().Format(time.RFC3339)
			cfg.Registries[i] = r
			if err := m.writeRegistries(cfg); err != nil {
				return r, false, err
			}
		}
		return r, changed, nil
	}
	return Registry{}, false, fmt.Errorf("%w: %q", ErrRegistryMissing, alias)
}

// RefreshAll refreshes every git registry in declaration order,
// collecting per-registry failures instead of stopping at the first
// one.
func (m Manager) RefreshAll(ctx context.Context) ([]Registry, []string, []error) {
	cfg, err := m.Load(ctx)
	if err != nil {
		return nil, nil, []error{err}
	}
	var updated []Registry
	var skipped []string
	var errs []error
	for _, r := range cfg.Registries {
		if r.Kind == KindLocal {
			continue
		}
		reg, changed, err := m.Refresh(ctx, r.Alias)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", r.Alias, err))
			continue
		}
		if changed {
			updated = append(updated, reg)
		} else {
			skipped = append(skipped, r.Alias)
		}
	}
	return updated, skipped, errs
}

// Bootstrap registers the official registry on first run, when
// registries.json does not exist yet. It never overwrites user data:
// an existing file (even corrupt) means no-op. Concurrent processes
// are serialized via a lock file with a TTL so a crashed process
// cannot block later boots forever. It reports whether the official
// registry was added by this call.
func (m Manager) Bootstrap(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if _, err := os.Stat(m.RegistriesPath()); err == nil || !os.IsNotExist(err) {
		return false, nil
	}
	// Serialize first-run registration so two concurrent spin
	// processes cannot both clone the official registry.
	lockPath := filepath.Join(m.CacheDir, ".bootstrap.lock")
	if err := os.MkdirAll(m.CacheDir, 0o755); err != nil {
		return false, err
	}
	claimed, err := acquireBootstrapLock(lockPath)
	if err != nil {
		return false, err
	}
	if !claimed {
		return false, nil // another process is bootstrapping
	}
	defer releaseBootstrapLock(lockPath)
	// Double-checked locking: the winning process may have finished
	// while we were acquiring the claim.
	if _, err := os.Stat(m.RegistriesPath()); err == nil || !os.IsNotExist(err) {
		return false, nil
	}
	if _, err := m.Add(ctx, OfficialAlias, DefaultRegistryURL, false); err != nil {
		return false, err
	}
	return true, nil
}

// bootstrapLockTTL is how long a .bootstrap.lock claim may stand
// before a later process treats it as stale. Generous so a slow
// clone is never misread as stale.
const bootstrapLockTTL = 2 * time.Minute

// acquireBootstrapLock atomically claims lockPath with O_CREATE|O_EXCL.
// A fresh claim held elsewhere yields false. A stale claim older than
// bootstrapLockTTL is removed and re-claimed.
func acquireBootstrapLock(lockPath string) (bool, error) {
	for {
		lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_ = lock.Close()
			return true, nil
		}
		if !os.IsExist(err) {
			return false, err
		}
		info, err := os.Stat(lockPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // released between OpenFile and Stat
			}
			return false, err
		}
		if time.Since(info.ModTime()) < bootstrapLockTTL {
			return false, nil
		}
		_ = os.Remove(lockPath)
	}
}

// releaseBootstrapLock drops a claim held by this process. A leaked
// claim expires via bootstrapLockTTL.
func releaseBootstrapLock(lockPath string) {
	_ = os.Remove(lockPath)
}

// Remove drops alias from registries.json and deletes its cache
// directory. When pinned templates depend on the registry, Remove
// fails unless purgePinned also soft-deletes those pins.
func (m Manager) Remove(ctx context.Context, alias string, pinnedTemplates []Pinned, purgePinned bool) error {
	cfg, err := m.Load(ctx)
	if err != nil {
		return err
	}
	var match Registry
	found := false
	out := make([]Registry, 0, len(cfg.Registries))
	for _, r := range cfg.Registries {
		if r.Alias == alias {
			match = r
			found = true
			continue
		}
		out = append(out, r)
	}
	if !found {
		return fmt.Errorf("%w: %q", ErrRegistryMissing, alias)
	}

	dependents := m.findDependentPins(match, pinnedTemplates)
	if len(dependents) > 0 && !purgePinned {
		names := make([]string, len(dependents))
		for i, p := range dependents {
			names[i] = p.Name
		}
		return fmt.Errorf("%q is used by pinned templates: %s (run with --purge-pinned to drop them first)",
			alias, strings.Join(names, ", "))
	}

	if err := m.writeRegistries(RegistriesConfig{Registries: out}); err != nil {
		return err
	}

	if match.Path != "" {
		// RemoveAll on a symlink deletes the link itself, not the target.
		if err := os.RemoveAll(match.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete cache %s: %w", match.Path, err)
		}
	}

	// Soft-delete dependent pins after unlinking so a pin-write
	// failure cannot leave a registry entry pointing at a deleted
	// cache.
	for _, p := range dependents {
		_ = (&Client{CacheDir: m.CacheDir}).Unpin(ctx, p.Name)
	}
	return nil
}

// findDependentPins returns the pins cloned from this registry's
// templates, or whose cache lives inside a local registry's path.
func (m Manager) findDependentPins(reg Registry, pinned []Pinned) []Pinned {
	if reg.Path == "" {
		return nil
	}
	// Collect template source URLs declared by this registry.
	templateSources := make(map[string]bool)
	tplDir := filepath.Join(reg.Path, "templates")
	if entries, err := os.ReadDir(tplDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".toml" {
				continue
			}
			var tpl TemplateMetadata
			if _, err := toml.DecodeFile(filepath.Join(tplDir, e.Name()), &tpl); err != nil {
				continue
			}
			if tpl.Source != "" {
				templateSources[tpl.Source] = true
			}
		}
	}

	var out []Pinned
	for _, p := range pinned {
		if p.Source != "" && templateSources[p.Source] {
			out = append(out, p)
			continue
		}
		if reg.Kind == KindLocal && p.LocalPath != "" {
			if rel, err := filepath.Rel(reg.Path, p.LocalPath); err == nil && !strings.HasPrefix(rel, "..") {
				out = append(out, p)
			}
		}
	}
	return out
}

// writeRegistries persists the registry catalogue atomically.
func (m Manager) writeRegistries(cfg RegistriesConfig) error {
	return atomicWriteJSON(m.RegistriesPath(), cfg, ".registries-*.json.tmp")
}
