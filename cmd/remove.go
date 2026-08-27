package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sam0uly/spin/internal/registry"
)

var removeCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove a pinned template",
	Long:    "Remove a template from the pinned list. The on-disk cache is left alone unless --purge is passed.",
	Example: `  # Remove a pin (cache kept on disk for reuse)
  spin remove go-cli

  # Remove a pin AND delete its on-disk cache
  spin remove go-cli --purge`,
	Args:          cobra.ExactArgs(1),
	RunE:          runRemove,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var removePurgeFlag bool

func init() {
	removeCmd.Flags().BoolVar(&removePurgeFlag, "purge", false, "also delete the on-disk cache (default: keep it for future `spin add`)")
	rootCmd.AddCommand(removeCmd)
}

// runRemove implements `spin remove`: soft-delete by default, or drop
// the record and its cache with --purge. Soft-deleted entries are
// still visible here so a later --purge can find them.
func runRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	client := registry.New()
	all, err := client.ListAllPinned(cmd.Context())
	if err != nil {
		return err
	}
	var match *registry.Pinned
	for i := range all {
		if all[i].Name == name {
			match = &all[i]
			break
		}
	}
	if match == nil {
		return fmt.Errorf("no pinned template named %q (run `spin list`)", name)
	}
	if removePurgeFlag {
		if err := client.Purge(cmd.Context(), name); err != nil {
			return err
		}
		if match.LocalPath != "" {
			printSuccess("removed %q and deleted cache at %s", name, match.LocalPath)
		} else {
			printSuccess("removed %q (no on-disk cache to purge)", name)
		}
		return nil
	}
	if err := client.Unpin(cmd.Context(), name); err != nil {
		return err
	}
	if match.LocalPath != "" {
		printSuccess("removed %q (cache kept at %s; run `spin remove %q --purge` to delete)", name, match.LocalPath, name)
	} else {
		printSuccess("removed %q (no on-disk cache)", name)
	}
	return nil
}
