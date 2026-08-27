package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sam0uly/spin/internal/registry"
)

var addCmd = &cobra.Command{
	Use:   "add <spec>",
	Short: "Pin a template locally for offline use",
	Long:  "Pin a template (local path or git URL) so `spin new` can use it offline. Pinned templates are stored in ~/.config/spin/pinned.json and cached under ~/.config/spin/templates/.",
	Example: `  # Pin from a local path (no network)
  spin add ~/code/templates/go-cli

  # Pin from a git URL (shallow-cloned, GIT_TERMINAL_PROMPT=0)
  spin add https://github.com/me/go-cli-template.git

  # List pinned templates
  spin add --list
`,
	Args:          cobra.MinimumNArgs(0),
	RunE:          runAdd,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var addListFlag bool

func init() {
	addCmd.Flags().BoolVar(&addListFlag, "list", false, "show pinned templates and exit")
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	client := registry.New()

	// `--list` (or no args) prints the pinned list and exits.
	if addListFlag || len(args) == 0 {
		return execList(cmd, nil)
	}

	spec := strings.TrimSpace(args[0])

	// <alias>/<id> shorthand: resolve through the registry, then pin
	// the resolved source as if typed directly. Name stays the template
	// id so `spin new <name>` keeps working.
	if registry.IsShorthand(spec) {
		mgr := registry.NewManager()
		maybeBootstrapOfficial(ctx, mgr)
		resolved, err := mgr.ResolveShorthand(ctx, spec)
		if err != nil {
			if enriched, ok := annotateShorthandError(err); ok {
				return enriched
			}
			alias, _ := registry.SplitAliasID(spec)
			return fmt.Errorf("alias %q is not a registered registry; use a full git URL instead", alias)
		}
		pinned, err := client.Add(ctx, resolved.Source)
		if err != nil {
			return err
		}
		pinned.Name = resolved.ID
		if err := client.Pin(ctx, *pinned); err != nil {
			return err
		}
		kind := "cloned from"
		if pinned.Version == "local" {
			kind = "local at"
		}
		printSuccess("added %s (%s %s, resolved from %s)", pinned.Name, kind, pinned.LocalPath, spec)
		return nil
	}

	pinned, err := client.Add(ctx, spec)
	if err != nil {
		return err
	}

	if err := client.Pin(ctx, *pinned); err != nil {
		return err
	}

	kind := "cloned to"
	if pinned.Version == "local" {
		kind = "local at"
	}
	printSuccess("added %s (%s %s)", pinned.Name, kind, pinned.LocalPath)
	return nil
}
