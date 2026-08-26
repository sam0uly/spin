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

// Client is the local pin store. It owns ~/.config/spin/pinned.json
// and the per-template caches under ~/.config/spin/templates/. The
// v2.x registry layer (manager.go) owns a separate registries store
// at ~/.config/spin/registries.json plus per-registry clones under
// ~/.config/spin/registries/<alias>/.
type Client struct {
	CacheDir string // where Pinned entries are stored; defaults to ~/.config/spin/pinned.json
}

func New() *Client {
	return &Client{CacheDir: defaultConfigDir()}
}

// Add resolves a spec (local path or git URL) into a Pinned
// template. The clone/copy runs before the Pinned record is
// returned, so the caller writes pinned.json only on success.
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

// validateTemplateDir checks that dir contains spin.toml and _base/.
// Used by addLocal and addGit to reject non-template directories
// at pin time rather than deferring the error to `spin new`.
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
	// Resolve to absolute so the symlink target survives when
	// followed from any directory (the cache lives under
	// ~/.config/spin/templates/, not the user's cwd).
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

	// Remove any previous pin of this name so the symlink/copy is
	// fresh. (Pin-de-dupe is a separate concern, handled in Pin().)
	if err := os.RemoveAll(dest); err != nil {
		return nil, fmt.Errorf("clear dest: %w", err)
	}

	// Try symlink first (cheap, no copy). On Windows without
	// SeCreateSymbolicLinkPrivilege, or on filesystems that don't
	// support symlinks, fall back to a recursive copy.
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

	// Remove any previous clone.
	if err := os.RemoveAll(dest); err != nil {
		return nil, fmt.Errorf("clear dest: %w", err)
	}

	// Shallow clone, no terminal prompts. GIT_TERMINAL_PROMPT=0 keeps
	// a missing credential from blocking on a password prompt.
	if err := GitClone(ctx, spec, dest); err != nil {
		return nil, err
	}
	if err := validateTemplateDir(dest); err != nil {
		_ = os.RemoveAll(dest)
		return nil, err
	}

	// Best-effort: capture the resolved HEAD sha so a refresh can
	// see if upstream has moved. Not fatal if git is missing or
	// the clone has no commits.
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

// expandHome returns path with a leading "~" or "~/" expanded to
// the user's home directory. Pure-Go; no shell.
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

// CopyTreeForTest is a test-only helper that exposes copyDir
// outside the package. The leading lowercase `c` would normally
// stay unexported; cmd/update_test.go uses this to seed the
// pinned LocalPath with a known-good initial copy. Production
// code should call (c *Client).Refresh or (c *Client).Add.
func CopyTreeForTest(src, dst string) error { return copyDir(src, dst) }

// SanitiseRepoName extracts the repo basename from a git URL. E.g.
//
//	"https://github.com/foo/bar.git"  -> "bar"
//	"git@github.com:foo/bar.git"      -> "bar"
//	"github.com/foo/bar"              -> "bar"
//
// The result is used as a directory name under the cache dir, so it
// must be safe across filesystems: lowercase, no scheme, no .git
// suffix. The function is the single source of truth for this
// transformation; both this package and internal/template call it
// to keep cache layout consistent.
func SanitiseRepoName(rawURL string) string {
	base := rawURL
	// Drop the scheme / protocol prefix so we can find the last "/"
	// or ":" separator.
	for _, prefix := range []string{"https://", "http://", "git://", "ssh://"} {
		if after, ok := strings.CutPrefix(base, prefix); ok {
			base = after
			break
		}
	}
	base = strings.TrimPrefix(base, "git@")
	// For scp-style URLs ("git@host:owner/repo.git") the colon
	// separates host from path.
	if i := strings.LastIndexAny(base, "/:"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".git")
	return base
}

// ─── pinned templates (local state) ───────────────────────────────

func (c *Client) PinnedPath() string {
	return filepath.Join(c.CacheDir, "pinned.json")
}

// Returns every persisted Pinned entry, including soft-deleted ones.
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

func (c *Client) Pin(ctx context.Context, p Pinned) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return err
	}
	// Default LocalPath for older callers that pre-date the field.
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

// Refresh re-clones (or re-copies) the on-disk cache for a pinned
// template in place, then updates the pin record's Version with the
// newly resolved HEAD SHA (or "local" for local-path sources). The
// LocalPath is preserved so any code that referenced it by path
// still works. If the pin has gone missing on disk, the user is
// told to run `spin add` again rather than getting a half-built
// clone back.
//
// `pin` is passed by value so callers can decide whether to keep
// the returned record (call Pin with it) or just inspect it.
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

	// Branch on source kind. Local paths are re-copied in place
	// (cheap, no network). Git URLs re-clone on top of the existing
	// dir -- `git fetch` would also work, but a full re-clone is
	// simpler and matches the freshness the user expects.
	//
	// Note: we do NOT require pin.LocalPath to exist. `cmd.update`
	// moves it aside to a .bak snapshot for rollback; from Refresh's
	// point of view a missing LocalPath is just a fresh clone.
	switch {
	case srcspec.IsLocalPath(pin.Source):
		// Re-copy from source. If the source is gone, fail so the
		// user re-pins rather than keeping a stale copy.
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

// writePinned writes the pinned list atomically: marshal to JSON,
// write to a sibling temp file, fsync, then rename over the real
// file. This prevents a partial write (e.g. process killed) from
// leaving pinned.json in a corrupt state.
func (c *Client) writePinned(all []Pinned) error {
	return atomicWriteJSON(c.PinnedPath(), all, ".pinned-*.json.tmp")
}
