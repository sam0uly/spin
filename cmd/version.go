package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"samouly.fun/spin/internal/version"
)

var versionCmd = &cobra.Command{
	Use:           "version",
	Short:         "Print the spin version",
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, args []string) { fmt.Println(version.Version) },
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
