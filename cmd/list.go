package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sam0uly/spin/internal/registry"
	"github.com/sam0uly/spin/internal/template"
)

var listCmd = &cobra.Command{
	Use:           "list",
	Short:         "List pinned templates",
	Long:          "List every template pinned locally via `spin add`. Pinned templates are stored in ~/.config/spin/pinned.json and cached under ~/.config/spin/templates/.",
	Args:          cobra.NoArgs,
	RunE:          execList,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var listJSONFlag bool
var listAllFlag bool

func init() {
	listCmd.Flags().BoolVar(&listJSONFlag, "json", false, "emit pinned templates as JSON to stdout (machine-readable, no styling)")
	listCmd.Flags().BoolVar(&listAllFlag, "all", false, "include removed pins (marked with `(removed)` next to the name)")
	rootCmd.AddCommand(listCmd)
}

// pinnedRow is the JSON-friendly, user-facing view of a pin. It is
// deliberately separate from registry.Pinned so the on-disk schema
// can evolve without breaking script consumers.
type pinnedRow struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	PinnedAt    string `json:"pinned_at,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"`
	LocalPath   string `json:"local_path"`
	Removed     bool   `json:"removed,omitempty"`
}

// execList prints pinned templates as a styled table, or as JSON with
// --json.
func execList(cmd *cobra.Command, args []string) error {
	client := registry.New()
	var pinned []registry.Pinned
	var err error
	if listAllFlag {
		pinned, err = client.ListAllPinned(cmd.Context())
	} else {
		pinned, err = client.ListPinned(cmd.Context())
	}
	if err != nil {
		return err
	}
	if len(pinned) == 0 {
		if listJSONFlag {
			return json.NewEncoder(cmd.OutOrStdout()).Encode([]any{})
		}
		if listAllFlag {
			printInfo("no pinned templates (active or removed)")
		} else {
			printInfo("no pinned templates")
		}
		printHint("use `spin add <spec>` to pin one (local path or git URL)")
		return nil
	}

	rows := make([]pinnedRow, 0, len(pinned))
	for _, p := range pinned {
		name := p.Name
		if p.Removed {
			name = p.Name + " (removed)"
		}
		rows = append(rows, pinnedRow{
			Name:        name,
			Version:     p.Version,
			PinnedAt:    p.PinnedAt,
			Description: pinnedDescription(p.LocalPath),
			Source:      p.Source,
			LocalPath:   p.LocalPath,
			Removed:     p.Removed,
		})
	}

	if listJSONFlag {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	headers := []string{"NAME", "VERSION", "PINNED", "DESCRIPTION", "LOCAL PATH"}
	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		tableRows = append(tableRows, []string{
			r.Name,
			r.Version,
			r.PinnedAt,
			r.Description,
			shortenLocal(r.LocalPath, client.CacheDir),
		})
	}
	printTable(io.Writer(cmd.OutOrStdout()), headers, shrinkCol(headers, tableRows))
	printHint("use `spin new <project> --template <name>` to scaffold from a pinned template")
	return nil
}

// pinnedDescription returns the Description from the spin.toml at
// localPath; a missing or malformed manifest yields "" rather than
// failing the listing.
func pinnedDescription(localPath string) string {
	if localPath == "" {
		return ""
	}
	st, err := template.ParseSpinToml(filepath.Join(localPath, "spin.toml"))
	if err != nil {
		return ""
	}
	return st.Description
}

// shortenLocal abbreviates the user's home directory to ~ when
// possible; empty paths render as "(unknown)".
func shortenLocal(p, _ string) string {
	if p == "" {
		return "(unknown)"
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}
