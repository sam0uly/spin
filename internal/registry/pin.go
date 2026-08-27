package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	srcspec "github.com/sam0uly/spin/internal/spec"
)

// Client owns the local pin store: pinned.json plus the per-template
// caches under CacheDir/templates.
type Client struct {
	CacheDir string // defaults to ~/.config/spin
}

// New returns a Client rooted at the default config dir.
func New() *Client {
	return &Client{CacheDir: defaultConfigDir()}
}

// Add resolves a spec (local path or git URL) into a Pinned record.
// The clone or copy happens before the record is returned, so callers
// can safely persist it only on success.
func (c *Client) Add(ctx context.Context, spec string) (*Pinned, error) {
	if spec == "" {
		return nil, fmt.Errorf("empty spec")
	}
	var pinned *Pinned
	var err error
	switch {
	case srcspec.IsLocalPath(spec):
		pinned, err = c.addLocal(ctx, spec)
	case srcspec.IsGitURL(spec):
		pinned, err = c.addGit(ctx, spec)
	default:
		return nil, fmt.Errorf("cannot resolve spec %q; expected a local path or git URL", spec)
	}
	if err != nil {
		return nil, err
	}
	pinned.PinnedAt = time.Now().UTC().Format(time.RFC3339)
	return pinned, nil
}

// validateTemplateDir reports whether dir looks like a template:
// it must contain spin.toml and _base/.
func validateTemplateDir(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "spin.toml")); err != nil {
		return fmt.Errorf("spin.toml not found in %s", dir)
	}
	if info, err := os.Stat(filepath.Join(dir, "_base")); err != nil || !info.IsDir() {
		return fmt.Errorf("_base/ directory not found in %s", dir)
	}
	return nil
}

func (c *Client) addLocal(ctx context.Context, spec string) (*Pinned, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	src, err := expandHome(spec)
	if err != nil {
		return nil, fmt.Errorf("add local: %w", err)
	}
	// Resolve to absolute so the symlink target works from any cwd.
	src, err = filepath.Abs(src)
	if err != nil {
		return nil, fmt.Errorf("add local: %w", err)
	}
	info, err := os.Stat(src)
	if err != nil {
		return nil, fmt.Errorf("add local: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", src)
	}
	if err := validateTemplateDir(src); err != nil {
		return nil, err
	}
	templatesDir := filepath.Join(c.CacheDir, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir templates: %w", err)
	}
	dest := filepath.Join(templatesDir, filepath.Base(src))

	// Drop any previous pin cache of this name.
	if err := os.RemoveAll(dest); err != nil {
		return nil, fmt.Errorf("clear dest: %w", err)
	}

	// Symlink when possible; fall back to a recursive copy on
	// filesystems without symlink support (e.g. Windows).
	if err := os.Symlink(src, dest); err != nil {
		if copyErr := copyDir(src, dest); copyErr != nil {
			return nil, fmt.Errorf("symlink (%v) and copy (%w) both failed", err, copyErr)
		}
	}

	return &Pinned{
		Name:      filepath.Base(src),
		Source:    src,
		Version:   "local",
		LocalPath: dest,
	}, nil
}

func (c *Client) addGit(ctx context.Context, spec string) (*Pinned, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	templatesDir := filepath.Join(c.CacheDir, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir templates: %w", err)
	}
	dest := filepath.Join(templatesDir, SanitiseRepoName(spec))

	// Drop any previous clone.
	if err := os.RemoveAll(dest); err != nil {
		return nil, fmt.Errorf("clear dest: %w", err)
	}

	if err := GitClone(ctx, spec, dest); err != nil {
		return nil, err
	}
	if err := validateTemplateDir(dest); err != nil {
		_ = os.RemoveAll(dest)
		return nil, err
	}

	// Best-effort HEAD sha so a later refresh can detect upstream
	// movement.
	version := "git"
	if sha, _ := gitHeadSHA(dest); sha != "" {
		version = sha
	}

	return &Pinned{
		Name:      filepath.Base(dest),
		Source:    spec,
		Version:   version,
		LocalPath: dest,
	}, nil
}

// expandHome expands a leading "~" or "~/" to the user's home
// directory.
func expandHome(path string) (string, error) {
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(h, path[2:]), nil
	}
	return path, nil
}

// copyDir recursively copies src to dst. Both must be directories.
// Used as a fallback when os.Symlink fails.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		}
		// Regular file: copy bytes + mode.
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// Re-create symlinks as symlinks.
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		dstFile, err := os.OpenFile(target, os.O_RDWR|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer dstFile.Close()
		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}

// gitHeadSHA returns the resolved HEAD sha for the repo at dir, or
// "" if the lookup fails (no git on PATH, empty repo, etc).
func gitHeadSHA(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(out))
	return s, nil
}

// CopyTreeForTest exposes copyDir to other packages for tests that
// seed a pin cache with known content.
func CopyTreeForTest(src, dst string) error { return copyDir(src, dst) }

// SanitiseRepoName extracts the repo basename from a git URL, e.g.
// "https://github.com/foo/bar.git" becomes "bar". The result is used
// as a directory name under the cache dir.
func SanitiseRepoName(rawURL string) string {
	base := rawURL
	for _, prefix := range []string{"https://", "http://", "git://", "ssh://"} {
		if after, ok := strings.CutPrefix(base, prefix); ok {
			base = after
			break
		}
	}
	base = strings.TrimPrefix(base, "git@")
	if i := strings.LastIndexAny(base, "/:"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".git")
	return base
}

// PinnedPath returns the location of pinned.json.
func (c *Client) PinnedPath() string {
	return filepath.Join(c.CacheDir, "pinned.json")
}

// ListAllPinned returns every persisted pin, including soft-deleted
// ones. A missing pinned.json yields a nil slice, not an error.
func (c *Client) ListAllPinned(ctx context.Context) ([]Pinned, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(c.PinnedPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return nil, nil
	}
	var out []Pinned
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("pinned.json: %w", err)
	}
	return out, nil
}

// ListPinned returns every pin that has not been soft-deleted.
func (c *Client) ListPinned(ctx context.Context) ([]Pinned, error) {
	all, err := c.ListAllPinned(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, x := range all {
		if !x.Removed {
			out = append(out, x)
		}
	}
	return out, nil
}

// Pin adds p to pinned.json, replacing an existing pin of the same
// name.
func (c *Client) Pin(ctx context.Context, p Pinned) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return err
	}
	// Older pins may pre-date the LocalPath field.
	if p.LocalPath == "" {
		p.LocalPath = filepath.Join(c.CacheDir, "templates", p.Name)
	}
	all, err := c.ListPinned(ctx)
	if err != nil {
		return err
	}
	for i, x := range all {
		if x.Name == p.Name {
			all[i] = p
			return c.writePinned(all)
		}
	}
	all = append(all, p)
	return c.writePinned(all)
}

// Unpin soft-deletes the named pin. Its cache stays on disk until
// Purge removes it. Unpinning an unknown name is not an error.
func (c *Client) Unpin(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	all, err := c.ListAllPinned(ctx)
	if err != nil {
		return err
	}
	found := false
	for i, x := range all {
		if x.Name == name {
			all[i].Removed = true
			found = true
		}
	}
	if !found {
		return nil
	}
	return c.writePinned(all)
}

// Purge deletes the named pin, its record, and its on-disk cache.
// It fails when no pin with that name exists.
func (c *Client) Purge(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	all, err := c.ListAllPinned(ctx)
	if err != nil {
		return err
	}
	var match Pinned
	found := false
	out := make([]Pinned, 0, len(all))
	for _, x := range all {
		if x.Name == name {
			match = x
			found = true
			continue
		}
		out = append(out, x)
	}
	if !found {
		return fmt.Errorf("no pinned template named %q", name)
	}
	if match.LocalPath != "" {
		if err := os.RemoveAll(match.LocalPath); err != nil {
			return fmt.Errorf("delete cache %s: %w", match.LocalPath, err)
		}
	}
	return c.writePinned(out)
}

// Refresh rebuilds the on-disk cache for a pin in place and updates
// its Version with the resolved HEAD sha (or "local" for local-path
// sources). The pin is passed by value; the caller decides whether
// to persist the returned record via Pin. A missing LocalPath is
// treated as a fresh clone, since cmd/update moves it aside to a
// .bak snapshot before calling Refresh.
func (c *Client) Refresh(ctx context.Context, pin Pinned) (Pinned, error) {
	if err := ctx.Err(); err != nil {
		return Pinned{}, err
	}
	if pin.Name == "" {
		return Pinned{}, fmt.Errorf("empty pin name")
	}
	if pin.LocalPath == "" {
		return Pinned{}, fmt.Errorf("pin %q has no LocalPath; re-run `spin add`", pin.Name)
	}

	switch {
	case srcspec.IsLocalPath(pin.Source):
		// Fail when the source is gone so the user re-pins instead of
		// keeping a stale copy.
		if _, err := os.Stat(pin.Source); err != nil {
			return Pinned{}, fmt.Errorf("source %s is gone: %w", pin.Source, err)
		}
		if err := os.RemoveAll(pin.LocalPath); err != nil {
			return Pinned{}, fmt.Errorf("clear %s: %w", pin.LocalPath, err)
		}
		if err := copyDir(pin.Source, pin.LocalPath); err != nil {
			return Pinned{}, fmt.Errorf("copy %s: %w", pin.Source, err)
		}
		if err := validateTemplateDir(pin.LocalPath); err != nil {
			return Pinned{}, err
		}
		pin.Version = "local"
	case srcspec.IsGitURL(pin.Source):
		if err := GitClone(ctx, pin.Source, pin.LocalPath); err != nil {
			return Pinned{}, err
		}
		if err := validateTemplateDir(pin.LocalPath); err != nil {
			return Pinned{}, err
		}
		pin.Version = "git"
		if sha, _ := gitHeadSHA(pin.LocalPath); sha != "" {
			pin.Version = sha
		}
	default:
		return Pinned{}, fmt.Errorf("%q has unknown source %q", pin.Name, pin.Source)
	}
	return pin, nil
}

// writePinned persists the pin list atomically.
func (c *Client) writePinned(all []Pinned) error {
	return atomicWriteJSON(c.PinnedPath(), all, ".pinned-*.json.tmp")
}
