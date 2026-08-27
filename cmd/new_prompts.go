package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/huh/v2"

	"github.com/sam0uly/spin/internal/log"
	"github.com/sam0uly/spin/internal/registry"
	"github.com/sam0uly/spin/internal/template"
	"github.com/sam0uly/spin/internal/theme"
)

// printHooks prints the template's hooks to stdout for --print-hooks.
func printHooks(tpl *template.Template) {
	if tpl.SpinToml == nil {
		return
	}
	if len(tpl.SpinToml.Pre) > 0 {
		log.Stdout.Print("  [[pre]] steps:")
		for _, s := range tpl.SpinToml.Pre {
			log.Stdout.Print(fmt.Sprintf("    run = %q", s.Run))
		}
	}
	if entries, _ := os.ReadDir(tpl.PreHookDir); len(entries) > 0 {
		log.Stdout.Print("  _pre/ files:")
		for _, e := range entries {
			if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				log.Stdout.Print("    " + filepath.Join("_pre", e.Name()))
			}
		}
	}
	if len(tpl.SpinToml.Post) > 0 {
		log.Stdout.Print("  [[post]] steps:")
		for _, s := range tpl.SpinToml.Post {
			log.Stdout.Print(fmt.Sprintf("    run = %q", s.Run))
		}
	}
	if entries, _ := os.ReadDir(tpl.PostHookDir); len(entries) > 0 {
		log.Stdout.Print("  _post/ files:")
		for _, e := range entries {
			if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				log.Stdout.Print("    " + filepath.Join("_post", e.Name()))
			}
		}
	}
}

// printResolvedParams prints template metadata and resolved values
// as JSON for --print-params.
func printResolvedParams(tpl *template.Template, values map[string]any) error {
	meta := map[string]any{
		"name": tpl.Name,
	}
	if tpl.SpinToml != nil {
		meta["description"] = tpl.SpinToml.Description
		meta["type"] = tpl.SpinToml.Type
		meta["language"] = tpl.SpinToml.Language
	}
	out := map[string]any{
		"template": meta,
		"values":   values,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// dryRunRender renders to memory, prints the file list that would be
// written, and writes nothing.
func dryRunRender(ctx context.Context, tpl *template.Template, values map[string]any, dest string) error {
	opts := template.HookOptions{NoHooks: true, PrintCommands: true}
	if len(tpl.SpinToml.Pre) > 0 {
		log.Debug("dry run: pre-hooks (skipped)")
		if err := template.RunPreHook(ctx, tpl, values, dest, opts); err != nil {
			return err
		}
	}
	files, err := tpl.Render(values)
	if err != nil {
		return err
	}
	printInfo("dry run: would write %d files to %s", len(files), dest)
	for path := range files {
		log.Stdout.Print("  " + filepath.Join(dest, path))
	}
	if len(tpl.SpinToml.Post) > 0 {
		log.Debug("dry run: post-hooks (skipped)")
		_ = template.RunPostHook(ctx, tpl, values, dest, opts)
	}
	return nil
}

// promptInvalidPinned is the Loader.PromptInvalidPinned hook.
// Asks "keep the broken clone, or remove it?"
func promptInvalidPinned(name, localPath string, detectErr error) (bool, error) {
	var keep bool
	form := theme.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Template %q is broken -- keep it?", name)).
				Description(fmt.Sprintf("%v", detectErr)).
				Value(&keep).
				Affirmative("Keep").
				Negative("Remove"),
		),
	)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return true, nil // abort means keep; never delete user data on cancel
		}
		return false, err
	}
	return keep, nil
}

// promptExistingDest is the Loader.PromptExistingDest hook.
// Reuse uses the existing clone as-is; Pin also persists the
// source for offline use; Wipe re-clones; Cancel aborts.
func promptExistingDest(name, localPath string) (template.DestAction, error) {
	var action string
	form := theme.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("%q already exists at %s", name, localPath)).
				Description("Reuse the existing clone, pin it for future offline use, wipe and re-clone, or cancel?").
				Options(
					huh.NewOption("Reuse existing clone (no network)", "reuse"),
					huh.NewOption("Pin existing clone (reuse + remember for offline)", "pin"),
					huh.NewOption("Wipe and re-clone", "wipe"),
					huh.NewOption("Cancel", "cancel"),
				).
				Value(&action),
		),
	)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return template.DestCancel, nil
		}
		return template.DestWipe, err
	}
	switch action {
	case "reuse":
		return template.DestReuse, nil
	case "pin":
		return template.DestPin, nil
	case "cancel":
		return template.DestCancel, nil
	default:
		return template.DestWipe, nil
	}
}

// promptPinAfterSuccess offers to pin a freshly-used remote
// template so future runs work offline. Skipped for local paths
// and already-pinned sources.
func promptPinAfterSuccess(ctx context.Context, _ string, tpl *template.Template) {
	client := registry.New()
	for _, p := range pinnedSnapshot(ctx, client) {
		if p.LocalPath == tpl.Source {
			return // already pinned
		}
	}
	var pin bool
	form := theme.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Pin %q for offline use?", tpl.Name)).
				Description(fmt.Sprintf("Source: %s\nFuture `spin new` calls can use --template %s without a network round-trip.", tpl.Repo, tpl.Name)).
				Value(&pin).
				Affirmative("Pin").
				Negative("Skip"),
		),
	)
	if err := form.Run(); err != nil {
		return // prompt failure is non-fatal; skip pinning
	}
	if !pin {
		return
	}
	pinned, err := client.Add(ctx, tpl.Repo)
	if err != nil {
		printHint("could not pin %q: %v", tpl.Repo, err)
		return
	}
	if err := client.Pin(ctx, *pinned); err != nil {
		printHint("could not save pin: %v", err)
		return
	}
	printSuccess("pinned %q (cloned to %s)", pinned.Name, pinned.LocalPath)
}

// pinnedSnapshot returns the pin list. Swallows read errors:
// "no pins" is the safe answer for a corrupt pinned.json.
func pinnedSnapshot(ctx context.Context, client *registry.Client) []registry.Pinned {
	pinned, err := client.ListPinned(ctx)
	if err != nil {
		return nil
	}
	return pinned
}

// promptOverwriteExisting asks "directory already exists, overwrite?" via huh.
// Returns true if the user confirms.
func promptOverwriteExisting(name string, dir string) bool {
	var overwrite bool
	form := theme.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Directory %q already exists", name)).
				Description(fmt.Sprintf("Contents of %s will be overwritten.\nDo you want to continue?", dir)).
				Value(&overwrite).
				Affirmative("Yes, overwrite").
				Negative("Cancel"),
		),
	)
	if err := form.Run(); err != nil {
		return false
	}
	return overwrite
}

// promptForName asks for the project name via a huh Input. Exits
// 130 on cancel.
func promptForName() (string, error) {
	var name string
	form := theme.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Description("Folder name; injected as {{ name }} / {{ project_name }} in the template.").
				Value(&name).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("name cannot be empty")
					}
					return nil
				}),
		),
	)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", ErrCancelled
		}
		return "", err
	}
	return strings.TrimSpace(name), nil
}

// promptForTemplate asks the user to pick a pinned template via a
// Select. Huh's Select has a built-in `/` filter, so the user
// narrows the list inline instead of answering a separate input.
// Returns a "no pinned templates" error when the cache is empty.
func promptForTemplate(ctx context.Context) (string, error) {
	client := registry.New()
	pinned, err := client.ListPinned(ctx)
	if err != nil {
		return "", err
	}
	if len(pinned) == 0 {
		return "", fmt.Errorf("spin new: no pinned templates; add a template link or path, or use `spin search` to find one")
	}

	names := make([]string, 0, len(pinned))
	for _, p := range pinned {
		names = append(names, p.Name)
	}
	slices.Sort(names)

	var choice string
	opts := make([]huh.Option[string], 0, len(names))
	for _, n := range names {
		opts = append(opts, huh.NewOption(n, n))
	}
	form := theme.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Pick a pinned template").
				Value(&choice).
				Options(opts...),
		),
	)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", ErrCancelled
		}
		return "", err
	}
	return choice, nil
}
