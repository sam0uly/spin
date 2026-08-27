package registry

// Pinned is a template saved for offline use by `spin add`. LocalPath
// is where the copy lives on disk. Old pin files from before v2.0 may
// have an empty LocalPath; consumers should fall back to
// CacheDir/templates/<name>.
type Pinned struct {
	Name      string `json:"name"`              // "vercel/nextjs-tailwind"
	Source    string `json:"source"`            // git URL or local path
	PinnedAt  string `json:"pinned_at"`         // ISO 8601 timestamp
	Version   string `json:"version"`           // version or commit at pin time
	LocalPath string `json:"local_path"`        // absolute cache path on disk
	Removed   bool   `json:"removed,omitempty"` // soft-deleted; cache stays until --purge
}

// RegistryKind describes how a registry was sourced.
type RegistryKind string

const (
	// KindGit is a registry cloned from a git URL. Refresh runs git
	// fetch and reset against the upstream.
	KindGit RegistryKind = "git"
	// KindLocal is a registry symlinked from a local path. Refresh is
	// a no-op because the user's filesystem is the source of truth.
	KindLocal RegistryKind = "local"
)

// Registry is one entry in registries.json. Alias is the shorthand
// used in `<alias>/<id>` references, Source is the spec given to
// `spin registry add`, and Path is the on-disk clone or symlink
// under CacheDir/registries/<alias>.
type Registry struct {
	Alias       string       `json:"alias"`
	Source      string       `json:"source"`
	Kind        RegistryKind `json:"kind"`
	Path        string       `json:"path"`
	AddedAt     string       `json:"added_at,omitempty"`
	LastUpdated string       `json:"last_updated,omitempty"`
}

// RegistriesConfig is the on-disk shape of registries.json.
type RegistriesConfig struct {
	Registries []Registry `json:"registries"`
}

// RegistryMetadata is the registry.toml schema. ID and Name are
// required; the rest is documentation for `spin search` output.
type RegistryMetadata struct {
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Homepage    string `toml:"homepage"`
	Maintainer  string `toml:"maintainer"`
	License     string `toml:"license"`
}

// TemplateMetadata is the schema of one templates/*.toml file inside
// a registry. ID is the short name users type in `<alias>/<id>`, and
// Source is the git URL or local path the template loader consumes.
type TemplateMetadata struct {
	ID          string   `toml:"id"`
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Source      string   `toml:"source"`
	Tags        []string `toml:"tags"`
	Authors     []string `toml:"authors"`
	License     string   `toml:"license"`
	Homepage    string   `toml:"homepage"`
	Type        string   `toml:"type"`
	Language    string   `toml:"language"`
	Version     string   `toml:"version"`
	UpdatedAt   string   `toml:"updated_at"`
}

// TemplateEntry is one search result: a registry alias plus its
// template metadata.
type TemplateEntry struct {
	Alias       string
	ID          string
	Name        string
	Description string
	Source      string
	Tags        []string
	Type        string
	Language    string
	Version     string
}
