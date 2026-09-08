package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"samouly.fun/spin/internal/registry"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage local template registries",
	Long:  "Register, list, update, and remove template registries. Registries are git repos or local paths that contain a registry.toml manifest and a templates/ directory of per-template metadata.",
}

var registryAddCmd = &cobra.Command{
	Use:   "add <alias> <source>",
	Short: "Register a new template registry",
	Long:  "Register a git URL or local path as a named registry. The alias is the user-facing shorthand used in `<alias>/<id>` references.",
	Example: `  spin registry add official https://github.com/spin-org/registry
  spin registry add local ~/work/registry
  spin registry add demo ./demo-registry --force`,
	Args:          cobra.ExactArgs(2),
	RunE:          runRegistryAdd,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var registryListCmd = &cobra.Command{
	Use:           "list",
	Short:         "List registered registries",
	Long:          "Print every registered registry with its source, kind, cache path, and template count. Use --json for scripting.",
	RunE:          runRegistryList,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var registryUpdateCmd = &cobra.Command{
	Use:   "update [alias]",
	Short: "Update one or all git registries",
	Long:  "Refresh one named registry, or every git registry when alias is omitted. Local registries are skipped (the user's filesystem is the source of truth).",
	Example: `  spin registry update
  spin registry update official
  spin registry update --quiet`,
	Args:          cobra.MaximumNArgs(1),
	RunE:          runRegistryUpdate,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var registryRemoveCmd = &cobra.Command{
	Use:   "remove <alias>",
	Short: "Remove a registered registry",
	Long:  "Remove a registry from registries.json and delete its cache directory. Refuses if pinned templates depend on the registry unless --purge-pinned is passed.",
	Example: `  spin registry remove official
  spin registry remove local --purge-pinned`,
	Args:          cobra.ExactArgs(1),
	RunE:          runRegistryRemove,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var (
	registryAddForce    bool
	registryUpdateQuiet bool
	registryRemovePurge bool
)

func init() {
	registryAddCmd.Flags().BoolVar(&registryAddForce, "force", false, "replace an existing alias (wipes its cache)")
	registryUpdateCmd.Flags().BoolVar(&registryUpdateQuiet, "quiet", false, "suppress per-registry output; print summary only")
	registryRemoveCmd.Flags().BoolVar(&registryRemovePurge, "purge-pinned", false, "also soft-delete pinned templates that depend on this registry")
	registryCmd.AddCommand(registryAddCmd, registryListCmd, registryUpdateCmd, registryRemoveCmd)
	rootCmd.AddCommand(registryCmd)
}

func runRegistryAdd(cmd *cobra.Command, args []string) error {
	alias, source := args[0], args[1]
	mgr := registry.NewManager()
	reg, err := mgr.Add(cmd.Context(), alias, source, registryAddForce)
	if err != nil {
		return err
	}
	kind := string(reg.Kind)
	printSuccess("registered %q (%s, cached at %s)", reg.Alias, kind, reg.Path)
	return nil
}

type registryRow struct {
	Alias          string `json:"alias"`
	Source         string `json:"source"`
	Kind           string `json:"kind"`
	Path           string `json:"path"`
	TemplatesCount int    `json:"templates_count"`
	AddedAt        string `json:"added_at,omitempty"`
	LastUpdated    string `json:"last_updated,omitempty"`
}

func runRegistryList(cmd *cobra.Command, args []string) error {
	mgr := registry.NewManager()
	regs, err := mgr.List(cmd.Context())
	if err != nil {
		return err
	}
	rows := make([]registryRow, 0, len(regs))
	for _, r := range regs {
		count := countTemplates(r.Path)
		rows = append(rows, registryRow{
			Alias:          r.Alias,
			Source:         r.Source,
			Kind:           string(r.Kind),
			Path:           r.Path,
			TemplatesCount: count,
			AddedAt:        r.AddedAt,
			LastUpdated:    r.LastUpdated,
		})
	}
	if registryListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}
	if len(rows) == 0 {
		printInfo("no registries registered; run `spin registry add <alias> <source>` to register one")
		return nil
	}
	headers := []string{"ALIAS", "SOURCE", "KIND", "TEMPLATES", "PATH"}
	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		tableRows = append(tableRows, []string{
			r.Alias,
			r.Source,
			r.Kind,
			fmt.Sprintf("%d", r.TemplatesCount),
			r.Path,
		})
	}
	printTable(os.Stdout, headers, tableRows)
	return nil
}

var registryListJSON bool

func init() {
	registryListCmd.Flags().BoolVar(&registryListJSON, "json", false, "machine-readable output")
}

// countTemplates returns the number of *.toml files under
// <registryPath>/templates/. Used by `spin registry list` so users
// can see how many templates each registry contributes. Returns 0 if
// the dir is missing (the registry was registered before this field
// was added).
func countTemplates(registryPath string) int {
	dir := filepath.Join(registryPath, "templates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".toml" {
			n++
		}
	}
	return n
}

func runRegistryUpdate(cmd *cobra.Command, args []string) error {
	mgr := registry.NewManager()
	if len(args) == 1 {
		alias := args[0]
		if _, ok := mgr.Get(cmd.Context(), alias); !ok {
			return fmt.Errorf("%q is not registered", alias)
		}
		reg, changed, err := mgr.Refresh(cmd.Context(), alias)
		if err != nil {
			return err
		}
		if reg.Kind == registry.KindLocal {
			printInfo("%s is local; nothing to update", alias)
			return nil
		}
		if !changed {
			if !registryUpdateQuiet {
				printInfo("%s already up to date", alias)
			}
			return nil
		}
		if !registryUpdateQuiet {
			printSuccess("updated %s (now at %s)", alias, reg.LastUpdated)
		}
		return nil
	}

	regs, skipped, errs := mgr.RefreshAll(cmd.Context())
	if len(regs) == 0 && len(skipped) == 0 && len(errs) == 0 {
		printInfo("no git registries to update")
		return nil
	}
	if !registryUpdateQuiet {
		for _, r := range regs {
			printSuccess("updated %s", r.Alias)
		}
		for _, alias := range skipped {
			printInfo("%s already up to date", alias)
		}
	}
	for _, e := range errs {
		printWarn("%v", e)
	}
	if len(errs) > 0 && len(regs) == 0 {
		return fmt.Errorf("all registries failed")
	}
	return nil
}

func runRegistryRemove(cmd *cobra.Command, args []string) error {
	alias := args[0]
	mgr := registry.NewManager()

	client := registry.New()
	pinned, err := client.ListAllPinned(cmd.Context())
	if err != nil {
		return err
	}
	if err := mgr.Remove(cmd.Context(), alias, pinned, registryRemovePurge); err != nil {
		if errors.Is(err, registry.ErrRegistryMissing) {
			return fmt.Errorf("%q is not registered", alias)
		}
		return err
	}
	if registryRemovePurge {
		printSuccess("removed %q and soft-deleted dependent pins", alias)
	} else {
		printSuccess("removed %q", alias)
	}
	return nil
}
