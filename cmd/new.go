package cmd

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/sam0uly/spin/internal/params"
	"github.com/sam0uly/spin/internal/registry"
	srcspec "github.com/sam0uly/spin/internal/spec"
	"github.com/sam0uly/spin/internal/template"
)

// ErrCancelled is returned when the user cancels an interactive prompt.
// The CLI exits with code 130 (SIGINT convention).
var ErrCancelled = errors.New("cancelled -- no project was created")

var newCmd = &cobra.Command{
	Use:   "new [<name>] [<template>]",
	Short: "Scaffold a new project from a template",
	Long:  "Scaffold a new project from an external template. A template is a git repo, local path, or pinned spec that contains a spin.toml manifest and a _base/ tree of overlay files. If <name> or <template> is omitted, spin prompts for it when running interactively. A single positional argument that looks like a template (path, git URL, or alias/id shorthand) is treated as the template.",
	Example: `  # Positional name + template (no flag needed)
  spin new myapp my-template
  spin new demoapp https://github.com/me/go-cli-template.git

  # Interactive: spin asks for the name (and template) when missing
  spin new

  # From a pinned template (recommended; works offline)
  spin new myapp --template my-template

  # From a local path
  spin new myapp --template ~/code/templates/go-cli

  # Non-interactive (CI / scripts): pre-set every param
  spin new myapp go-cli --param port=8080 --param api_key=... --param features=ci,release

  # Preview the template's params without scaffolding
  spin new myapp <spec> --print-params

  # Preview the rendered tree without writing files
  spin new myapp <spec> --dry-run`,
	Args:          validateNewArgs,
	RunE:          runNew,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var (
	newTemplate    string
	newDest        string
	newPrintParams bool
	newPrintHooks  bool
	newDryRun      bool
	newParams      []string
	newVerbose     bool
	newNoHooks     bool
	newYes         bool
)

func init() {
	newCmd.Flags().StringVarP(&newTemplate, "template", "t", "", "template spec: user/repo, git URL, or local path (or pass positionally as the second argument)")
	newCmd.Flags().StringVarP(&newDest, "dest", "d", "", "destination directory (default: ./<name>)")
	newCmd.Flags().BoolVar(&newPrintParams, "print-params", false, "print the template's params as JSON and exit (no files written)")
	newCmd.Flags().BoolVar(&newPrintHooks, "print-hooks", false, "print the template's hooks and exit (no files written)")
	newCmd.Flags().BoolVar(&newDryRun, "dry-run", false, "render to a temp dir, print the file list, and clean up (no project written)")
	newCmd.Flags().StringArrayVar(&newParams, "param", nil, "set a template param as key=value (repeatable); skips the interactive form. Use --print-params to discover valid keys.")
	newCmd.Flags().BoolVar(&newVerbose, "verbose", false, "print hook output while running")
	newCmd.Flags().BoolVar(&newNoHooks, "no-hooks", false, "skip pre and post hooks")
	newCmd.Flags().BoolVarP(&newYes, "yes", "y", false, "run template hooks without the trust confirmation prompt")
	rootCmd.AddCommand(newCmd)
}

// runNew is the RunE for `spin new`. Resolves name + template
// (positional / flag / interactive prompt), loads the template,
// collects params (interactive or via --param), renders the
// project, and runs [[post]] steps. Honors --print-params and
// --dry-run as preview-only short circuits.
func runNew(cmd *cobra.Command, args []string) error {
	name, tplSpec, err := resolveNameAndTemplate(cmd, args)
	if err != nil {
		return err
	}
	dest, err := resolveDest(newDest, name)
	if err != nil {
		return err
	}

	// Check if the destination already exists and has content.
	if dirHasFiles(dest) {
		if newYes {
			printWarn("directory %s already exists; overwriting", dest)
		} else if isInteractive() {
			if !promptOverwriteExisting(name, dest) {
				return fmt.Errorf("cancelled -- directory %s already exists", dest)
			}
		} else {
			return fmt.Errorf("destination %s already exists (use --yes to overwrite)", dest)
		}
	}

	loader := template.NewLoader("")
	if isInteractive() {
		loader.PromptInvalidPinned = promptInvalidPinned
		loader.PromptExistingDest = promptExistingDest
	}

	// If the spec is a registry shorthand and no registries are
	// configured yet, bootstrap the official registry so the
	// shorthand can resolve without manual setup.
	if registry.IsShorthand(tplSpec) {
		maybeBootstrapOfficial(cmd.Context(), registry.NewManager())
	}

	tpl, err := loader.LoadContext(cmd.Context(), tplSpec)
	if err != nil {
		if enriched, ok := annotateShorthandError(err); ok {
			return enriched
		}
		return err
	}

	values := map[string]any{
		"name":         name,
		"project_name": name,
	}
	if len(newParams) > 0 {
		parsed, err := applyParamFlags(tpl, newParams)
		if err != nil {
			return err
		}
		maps.Copy(values, parsed)
	}
	// --print-params, --dry-run, and --param all force
	// non-interactive so we never reask the user for answers they
	// already supplied.
	interactive := isInteractive() && !newPrintParams && !newPrintHooks && !newDryRun && len(newParams) == 0
	var resolved map[string]any
	renderedByTUI := false
	if interactive && tpl.SpinToml != nil && len(tpl.SpinToml.Params) > 0 {
		renderedByTUI, resolved, err = runNewTUI(tpl, values, cmd.Context(), dest, name, newNoHooks, newYes, newVerbose)
	} else {
		resolved, err = tpl.ResolveForm(values, interactive)
	}
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ErrCancelled
		}
		return err
	}

	if newPrintParams {
		return printResolvedParams(tpl, resolved)
	}
	if newPrintHooks {
		printHooks(tpl)
		return nil
	}
	if newDryRun {
		return dryRunRender(cmd.Context(), tpl, resolved, dest)
	}

	// When the TUI already ran the full scaffold (form + hook review),
	// don't render a second time.
	if renderedByTUI {
		printSuccess("created %s at %s", name, dest)
		printHint("cd %s", dest)
		if isInteractive() && tpl.Repo != "" {
			promptPinAfterSuccess(cmd.Context(), name, tpl)
		}
		return nil
	}

	opts := template.HookOptions{
		NoHooks:       newNoHooks,
		PrintCommands: true,
		Verbose:       newVerbose,
		Output:        stdoutHookOutput(),
		StepStart: func(kind, cmd string) {
			fmt.Fprint(os.Stdout, hookStepHeader(kind, cmd))
		},
	}
	if err := tpl.RenderToWithPost(cmd.Context(), dest, resolved, opts); err != nil {
		return err
	}

	printSuccess("created %s at %s", name, dest)
	printHint("cd %s", dest)

	if isInteractive() && tpl.Repo != "" {
		promptPinAfterSuccess(cmd.Context(), name, tpl)
	}
	return nil
}

// isInteractive reports whether stdin is a TTY. Drives the
// non-interactive path (--param, --print-params, --dry-run, CI).
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// resolveDest returns the absolute destination path. Expands
// ~ and ~/ via the user's home; everything else via filepath.Abs
// so the success line always shows a full path.
func resolveDest(dest, name string) (string, error) {
	if dest == "" {
		return filepath.Abs(name)
	}
	if dest == "~" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return h, nil
	}
	if strings.HasPrefix(dest, "~/") {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Abs(filepath.Join(h, dest[2:]))
	}
	return filepath.Abs(dest)
}

// dirHasFiles reports whether path exists and is a non-empty directory.
func dirHasFiles(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}

// applyParamFlags parses the repeated --param slice into a typed
// map keyed on the template's param spec. Unknown keys, malformed
// key=value, or out-of-range numbers produce clear errors naming
// the offending flag.
func applyParamFlags(tpl *template.Template, raw []string) (map[string]any, error) {
	out := make(map[string]any, len(raw))
	for i, entry := range raw {
		key, value, err := splitParamEntry(entry)
		if err != nil {
			return nil, fmt.Errorf("param %d %q: %v", i, entry, err)
		}
		spec, ok := tpl.SpinToml.Params[key]
		if !ok {
			return nil, fmt.Errorf("--param[%d] %q: unknown key %q (known: %s)", i, entry, key, joinKnownParams(tpl.SpinToml.Params))
		}
		coerced, err := coerceParamValue(spec, value)
		if err != nil {
			return nil, fmt.Errorf("param %d %q: %v", i, entry, err)
		}
		out[key] = coerced
	}
	return out, nil
}

// splitParamEntry parses one "key=value" string. Rejects empty
// key and missing '='.
func splitParamEntry(s string) (key, value string, err error) {
	key, value, ok := strings.Cut(s, "=")
	if !ok {
		return "", "", fmt.Errorf("missing '=' (expected key=value)")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", fmt.Errorf("empty key")
	}
	return key, value, nil
}

// coerceParamValue parses raw (always a string from the CLI) into
// the primitive the param type expects.
func coerceParamValue(spec params.Spec, raw string) (any, error) {
	switch spec.Type {
	case params.TypeNumber:
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("expected integer, got %q", raw)
		}
		if spec.Min != nil && n < *spec.Min {
			return nil, fmt.Errorf("%d is below min %d", n, *spec.Min)
		}
		if spec.Max != nil && n > *spec.Max {
			return nil, fmt.Errorf("%d is above max %d", n, *spec.Max)
		}
		return n, nil
	case params.TypeBool:
		b, err := parseLooseBool(raw)
		if err != nil {
			return nil, err
		}
		return b, nil
	case params.TypeMultiSelect:
		// Comma-split, trim each, drop empties so a trailing comma
		// or stray whitespace doesn't produce phantom options.
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out, nil
	case params.TypeSelect, params.TypeLicense:
		if len(spec.Options) > 0 && !slices.Contains(spec.Options, raw) {
			return nil, fmt.Errorf("not in options (want one of %v, got %q)", spec.Options, raw)
		}
		return raw, nil
	case params.TypeText, params.TypeTextarea, params.TypePath, params.TypeSecret, "":
		return raw, nil
	default:
		return nil, fmt.Errorf("unsupported param type %q", spec.Type)
	}
}

// parseLooseBool accepts truthy/falsy spellings: true/1/yes/y/on
// and false/0/no/n/off.
func parseLooseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "y", "on":
		return true, nil
	case "false", "0", "no", "n", "off":
		return false, nil
	}
	return false, fmt.Errorf("expected bool, got %q (use true/false, 1/0, yes/no)", s)
}

// joinKnownParams renders the param spec keys in stable order
// for the unknown-key error path.
func joinKnownParams(specs map[string]params.Spec) string {
	names := make([]string, 0, len(specs))
	for k := range specs {
		names = append(names, k)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// validateNewArgs is the cobra.Args validator. Accepts 0, 1, or 2
// positionals and rejects positional <template> + --template.
func validateNewArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 2 {
		return fmt.Errorf("accepts at most 2 positional args (<name> [<template>]), got %d", len(args))
	}
	if len(args) == 2 && cmd.Flags().Changed("template") {
		return fmt.Errorf("cannot pass <template> both positionally and via --template")
	}
	return nil
}

// resolveNameAndTemplate fills name and template from args /
// --template / interactive prompts. Returns precise non-interactive
// errors naming whichever slot is missing.
//
// A single positional argument that looks like a template spec
// (local path, git URL, or <alias>/<id> shorthand) is treated as the
// template; the user is then prompted for the project name. This
// matches the common "spin up this template" mental model.
func resolveNameAndTemplate(cmd *cobra.Command, args []string) (string, string, error) {
	var name, tpl string
	if len(args) == 1 && looksLikeTemplateSpec(args[0]) {
		tpl = args[0]
	} else {
		if len(args) >= 1 {
			name = args[0]
		}
		if len(args) >= 2 {
			tpl = args[1]
		}
	}
	// Validator already rejected positional + --template; prefer the
	// positional if both somehow arrive.
	if cmd.Flags().Changed("template") {
		if tpl != "" && newTemplate != "" && tpl != newTemplate {
			printWarn("ignoring --template %q in favor of positional %q", newTemplate, tpl)
		} else if tpl == "" {
			tpl = newTemplate
		}
	}

	if name == "" {
		if !isInteractive() {
			return "", "", fmt.Errorf("spin new: <name> is required in non-interactive mode")
		}
		v, err := promptForName()
		if err != nil {
			return "", "", err
		}
		name = v
	}
	if tpl == "" {
		if !isInteractive() {
			return "", "", fmt.Errorf("spin new: <template> is required in non-interactive mode (use `spin search <query>` to find one, or `spin list` for pinned)")
		}
		v, err := promptForTemplate(cmd.Context())
		if err != nil {
			return "", "", err
		}
		tpl = v
	}
	return name, tpl, nil
}

// looksLikeTemplateSpec reports whether s is unambiguously a template
// source: local path, git URL, or registry shorthand. Pinned names are
// intentionally excluded -- they are resolved later by the loader.
func looksLikeTemplateSpec(s string) bool {
	return srcspec.IsLocalPath(s) || srcspec.IsGitURL(s) || srcspec.IsShorthand(s)
}
