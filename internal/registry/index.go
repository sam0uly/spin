package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Index is a snapshot of every valid template under every registered
// registry. It is rebuilt on demand and not cached across runs; each
// search is fast enough that caching adds complexity for no gain.
type Index struct {
	entries []TemplateEntry
}

// Build scans every registry's templates/*.toml, parses and
// validates each file, and returns the resulting index plus a
// per-registry count of skipped files.
func (m Manager) Build(ctx context.Context) (*Index, map[string]int, error) {
	cfg, err := m.Load(ctx)
	if err != nil {
		return nil, nil, err
	}
	idx := &Index{}
	skipCounts := make(map[string]int)
	for _, reg := range cfg.Registries {
		tplDir := filepath.Join(reg.Path, "templates")
		entries, err := os.ReadDir(tplDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue // not yet populated: zero templates
			}
			skipCounts[reg.Alias]++
			continue
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".toml" {
				continue
			}
			var tpl TemplateMetadata
			if _, err := toml.DecodeFile(filepath.Join(tplDir, e.Name()), &tpl); err != nil {
				skipCounts[reg.Alias]++
				continue
			}
			if !validTemplate(&tpl, e.Name()) {
				skipCounts[reg.Alias]++
				continue
			}
			idx.entries = append(idx.entries, TemplateEntry{
				Alias:       reg.Alias,
				ID:          tpl.ID,
				Name:        tpl.Name,
				Description: tpl.Description,
				Source:      tpl.Source,
				Tags:        tpl.Tags,
				Type:        tpl.Type,
				Language:    tpl.Language,
				Version:     tpl.Version,
			})
		}
	}
	sort.Slice(idx.entries, func(i, j int) bool {
		return idx.entries[i].Alias+"/"+idx.entries[i].ID < idx.entries[j].Alias+"/"+idx.entries[j].ID
	})
	return idx, skipCounts, nil
}

// validTemplate reports whether tpl satisfies the registry template
// contract: non-empty id, name, and source, with id matching the
// file basename.
func validTemplate(tpl *TemplateMetadata, fileName string) bool {
	if tpl.ID == "" || tpl.Name == "" || tpl.Source == "" {
		return false
	}
	want := strings.TrimSuffix(fileName, ".toml")
	return tpl.ID == want
}

// Search returns entries matching query as a substring of alias,
// id, name, description, or tags, best matches first. An empty query
// returns everything in ascending alias/id order.
func (idx *Index) Search(query string, limit int) []TemplateEntry {
	scored := make([]scoredEntry, 0, len(idx.entries))
	q := strings.ToLower(query)
	for _, e := range idx.entries {
		s := score(e, q)
		if query != "" && s == 0 {
			continue
		}
		scored = append(scored, scoredEntry{entry: e, score: s})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		// Tie-break deterministically so equal-scored results keep a
		// stable order across runs.
		if scored[i].entry.ID != scored[j].entry.ID {
			return scored[i].entry.ID < scored[j].entry.ID
		}
		return scored[i].entry.Alias < scored[j].entry.Alias
	})
	out := make([]TemplateEntry, 0, len(scored))
	for _, s := range scored {
		out = append(out, s.entry)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// score rates how well an entry matches q. Higher is better; 0 means
// no match.
func score(e TemplateEntry, q string) int {
	if q == "" {
		return 1
	}
	id := strings.ToLower(e.Alias + "/" + e.ID)
	if id == q {
		return 100
	}
	if strings.Contains(id, q) {
		return 50
	}
	if strings.Contains(strings.ToLower(e.Name), q) {
		return 30
	}
	if strings.Contains(strings.ToLower(e.Description), q) {
		return 20
	}
	for _, t := range e.Tags {
		if strings.Contains(strings.ToLower(t), q) {
			return 10
		}
	}
	return 0
}

type scoredEntry struct {
	entry TemplateEntry
	score int
}

// Validate returns one message per problem found in a registry's
// metadata. An empty slice means the registry is fully valid.
func (m Manager) Validate(ctx context.Context, alias string) []string {
	reg, ok := m.Get(ctx, alias)
	if !ok {
		return []string{fmt.Sprintf("%s: not registered", alias)}
	}
	var out []string
	var rm RegistryMetadata
	if _, err := toml.DecodeFile(filepath.Join(reg.Path, "registry.toml"), &rm); err != nil {
		out = append(out, fmt.Sprintf("%s: registry.toml: %v", alias, err))
	} else {
		if rm.ID == "" {
			out = append(out, fmt.Sprintf("%s: registry.toml: missing id", alias))
		}
		if rm.Name == "" {
			out = append(out, fmt.Sprintf("%s: registry.toml: missing name", alias))
		}
	}
	tplDir := filepath.Join(reg.Path, "templates")
	entries, err := os.ReadDir(tplDir)
	if err != nil {
		out = append(out, fmt.Sprintf("%s: templates/: %v", alias, err))
		return out
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".toml" {
			continue
		}
		var tpl TemplateMetadata
		if _, err := toml.DecodeFile(filepath.Join(tplDir, e.Name()), &tpl); err != nil {
			out = append(out, fmt.Sprintf("%s/templates/%s: %v", alias, e.Name(), err))
			continue
		}
		if !validTemplate(&tpl, e.Name()) {
			out = append(out, fmt.Sprintf("%s/templates/%s: missing id/name/source or id mismatch", alias, e.Name()))
		}
	}
	return out
}
