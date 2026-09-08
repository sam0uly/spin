package cmd

import (
	"github.com/spf13/cobra"

	"samouly.fun/spin/internal/version"
)

// rootCmd is the cobra root command for spin.
var rootCmd = &cobra.Command{
	Use:                        "spin",
	Short:                      "Universal project scaffolder",
	Long:                       "spin scaffolds projects from external templates -- git repos, local paths, or pinned specs -- for any language or framework.",
	Version:                    version.Version,
	SilenceUsage:               true,
	SilenceErrors:              true,
	SuggestionsMinimumDistance: 2,
}

// RootCmd returns the spin cobra root command with all subcommands attached.
func RootCmd() *cobra.Command {
	return rootCmd
}
