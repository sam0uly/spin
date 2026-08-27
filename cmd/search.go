package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sam0uly/spin/internal/registry"
	"github.com/sam0uly/spin/internal/template"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search registries and pinned templates",
	Long:  "Search across templates in registered registries and pinned templates. Reads ~/.config/spin/registries/*/templates/*.toml and ~/.config/spin/pinned.json directly -- no network call once registries are registered. The official registry is added automatically on first run; register more with `spin registry add`.",
	Example: `  spin search "go cli"
  spin search rust --limit 5
  spin search tauri --json`,
	Args:          cobra.MinimumNArgs(1),
	RunE:          runSearch,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var (
	searchLimit int
	searchJSON  bool
)

func init() {
	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "max results to show")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "machine-readable output")
	rootCmd.AddCommand(searchCmd)
}

type searchEntry struct {
	Alias       string   `json:"alias"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Source      string   `json:"source"`
	Tags        []string `json:"tags,omitempty"`
	Type        string   `json:"type,omitempty"`
	Language    string   `json:"language,omitempty"`
	Version     string   `json:"version,omitempty"`
}

type searchResultView struct {
	Query   string        `json:"query"`
	Total   int           `json:"total"`
	Entries []searchEntry `json:"entries"`
}

// pinnedSearchEntries reads every active pinned template and returns
// a searchEntry for each one, populating metadata from the cached
// spin.toml when available.
func pinnedSearchEntries(ctx *cobra.Command, client *registry.Client, query string) []searchEntry {
	pinned, err := client.ListPinned(ctx.Context())
	if err != nil || len(pinned) == 0 {
		return nil
	}
	out := make([]searchEntry, 0, len(pinned))
	q := strings.ToLower(query)
	for _, p := range pinned {
		se := searchEntry{
			ID:      p.Name,
			Name:    p.Name,
			Source:  p.Source,
			Version: p.Version,
		}
		if p.LocalPath != "" {
			if st, err := template.ParseSpinToml(filepath.Join(p.LocalPath, "spin.toml")); err == nil {
				se.Description = st.Description
				se.Tags = st.Tags
				se.Type = st.Type
				se.Language = st.Language
			}
		}
		if query != "" {
			if !strings.Contains(strings.ToLower(se.ID), q) &&
				!strings.Contains(strings.ToLower(se.Name), q) &&
				!strings.Contains(strings.ToLower(se.Description), q) &&
				!strings.Contains(strings.ToLower(se.Type), q) &&
				!strings.Contains(strings.ToLower(se.Language), q) {
				tagMatch := false
				for _, tag := range se.Tags {
					if strings.Contains(strings.ToLower(tag), q) {
						tagMatch = true
						break
					}
				}
				if !tagMatch {
					continue
				}
			}
		}
		out = append(out, se)
	}
	return out
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	mgr := registry.NewManager()

	maybeBootstrapOfficial(cmd.Context(), mgr)

	idx, _, err := mgr.Build(cmd.Context())
	if err != nil {
		return err
	}
	results := idx.Search(query, searchLimit)

	// Merge pinned templates into results, deduplicating by source.
	// Merge pinned templates into results, deduplicating by source.
	client := registry.New()
	for _, pe := range pinnedSearchEntries(cmd, client, query) {
		dup := false
		for _, r := range results {
			if r.Source != "" && pe.Source != "" && r.Source == pe.Source {
				dup = true
				break
			}
			if pe.Source == "" && r.ID == pe.ID {
				dup = true
				break
			}
		}
		if !dup {
			results = append(results, registry.TemplateEntry{
				Alias:       pe.Alias,
				ID:          pe.ID,
				Name:        pe.Name,
				Description: pe.Description,
				Source:      pe.Source,
				Tags:        pe.Tags,
				Type:        pe.Type,
				Language:    pe.Language,
				Version:     pe.Version,
			})
		}
	}

	view := searchResultView{Query: query, Total: len(results), Entries: make([]searchEntry, 0, len(results))}
	for _, e := range results {
		view.Entries = append(view.Entries, searchEntry{
			Alias:       e.Alias,
			ID:          e.ID,
			Name:        e.Name,
			Description: e.Description,
			Source:      e.Source,
			Tags:        e.Tags,
			Type:        e.Type,
			Language:    e.Language,
			Version:     e.Version,
		})
	}

	if searchJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(view)
	}
	if view.Total == 0 {
		printInfo("no templates matched %q", query)
		printHint("run `spin registry add <alias> <git-or-local-source>` to register a registry, or `spin add <spec>` to pin one directly")
		return nil
	}
	headers := []string{"ALIAS/ID", "NAME", "LANG", "DESCRIPTION"}
	rows := make([][]string, 0, len(results))
	for _, e := range results {
		label := e.ID
		if e.Alias != "" {
			label = e.Alias + "/" + e.ID
		}
		rows = append(rows, []string{
			label,
			e.Name,
			e.Language,
			e.Description,
		})
	}
	printTable(os.Stdout, headers, shrinkCol(headers, rows))
	printHint("use `spin add <name>` to pin a template, or `spin add <alias>/<id>` from a registry")
	return nil
}
