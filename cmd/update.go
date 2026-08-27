package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/sam0uly/spin/internal/log"
	"github.com/sam0uly/spin/internal/registry"
)

var updateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Refresh a pinned template (or all, if name is omitted)",
	Long:  "Re-clone or re-copy the on-disk cache of a pinned template so the next `spin new` sees the latest spin.toml and _base/ tree. If name is omitted, every pinned template is refreshed.",
	Example: `  # Refresh one pinned template
  spin update go-cli

  # Refresh every pinned template
  spin update`,
	Args:          cobra.MaximumNArgs(1),
	RunE:          runUpdate,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

// runUpdate drives `spin update [name]`. Iterates either over the
// single named pin or over all of them, calling refreshOne (which
// does the rollback-aware clone/copy + version bump + Pin write)
// for each.
func runUpdate(cmd *cobra.Command, args []string) error {
	client := registry.New()
	pinned, err := client.ListPinned(cmd.Context())
	if err != nil {
		return err
	}
	if len(pinned) == 0 {
		printInfo("no pinned templates to update")
		printHint("use `spin add <spec>` to pin one (local path or git URL)")
		return nil
	}

	// Filter to the named pin (if given) or to all. An unknown
	// name is an error, not a silent no-op, so the user notices a
	// typo before assuming "it ran".
	var targets []registry.Pinned
	if len(args) == 1 {
		name := args[0]
		for _, p := range pinned {
			if p.Name == name {
				targets = []registry.Pinned{p}
				break
			}
		}
		if len(targets) == 0 {
			return fmt.Errorf("no pinned template named %q (run `spin list`)", name)
		}
	} else {
		targets = pinned
	}

	var ok, failed, skipped int
	for _, p := range targets {
		oldVersion := p.Version
		updated, err := refreshOne(cmd.Context(), client, p)
		if err != nil {
			printWarn("%s: %v", p.Name, err)
			failed++
			continue
		}
		short := updated.Version
		if len(short) > 10 {
			short = short[:10]
		}
		if oldVersion == updated.Version {
			printInfo("%s already up to date (%s)", updated.Name, short)
			skipped++
			continue
		}
		printSuccess("updated %s -> %s", updated.Name, short)
		ok++
	}
	if failed > 0 {
		log.Error("template refresh finished with failures", "updated", ok, "failed", failed)
		return fmt.Errorf("%d template(s) failed to refresh", failed)
	}
	switch {
	case ok == 0 && skipped > 0:
		log.Stdout.Info(fmt.Sprintf("%d template(s) already up to date", skipped))
	case ok == 1:
		log.Stdout.Print("1 template refreshed")
	default:
		log.Stdout.Info(fmt.Sprintf("%d templates refreshed", ok))
	}
	return nil
}

// refreshOne refreshes one pin's cache with rollback: the old cache
// is renamed aside first, moved back on failure, and deleted on
// success. The returned pin carries the new Version; persist it with
// client.Pin. A failed rollback is reported as a warning, not returned,
// since the original error is the actionable one.
func refreshOne(ctx context.Context, client *registry.Client, p registry.Pinned) (registry.Pinned, error) {
	if p.LocalPath == "" {
		return p, fmt.Errorf("no LocalPath on pin; re-run `spin add %s`", p.Source)
	}
	// Snapshot the old cache via rename (atomic on one filesystem,
	// and no second copy of a potentially huge cache). If the cache
	// is missing there is nothing to roll back to; Refresh rejects
	// that case itself. A cross-filesystem rename failure is recorded
	// but does not stop the update attempt.
	backup, haveBackup := backupPath(p.LocalPath)
	var backupErr error
	if _, err := os.Stat(p.LocalPath); err == nil {
		if err := os.Rename(p.LocalPath, backup); err != nil {
			backupErr = err
		} else {
			haveBackup = true
		}
	}

	updated, err := client.Refresh(ctx, p)
	if err != nil {
		if haveBackup {
			if rbErr := os.Rename(backup, p.LocalPath); rbErr != nil {
				printWarn("rollback also failed for %s: original=%v rollback=%v (backup at %s)",
					p.Name, err, rbErr, backup)
			}
		} else if backupErr != nil {
			printWarn("could not back up %s before refresh; update left old cache in place: %v",
				p.Name, backupErr)
		}
		return p, err
	}

	// Persist last so a pin-write failure cannot leave a refreshed
	// cache behind an outdated Version; roll the cache back instead.
	if err := client.Pin(ctx, updated); err != nil {
		if haveBackup {
			if rbErr := os.Rename(backup, p.LocalPath); rbErr != nil {
				printWarn("pin write failed for %s AND rollback failed: pin-err=%v rollback-err=%v (backup at %s)",
					p.Name, err, rbErr, backup)
			}
		}
		return p, fmt.Errorf("pin write failed: %v", err)
	}

	// All green: drop the snapshot.
	if haveBackup {
		if rmErr := os.RemoveAll(backup); rmErr != nil {
			log.Debug("failed to remove update backup", "path", backup, "err", rmErr)
		}
	}
	return updated, nil
}

// backupPath returns the rollback snapshot path for localPath:
// the original plus a ".bak-<unix-ts>" suffix so back-to-back updates
// never clobber each other's snapshots.
func backupPath(localPath string) (string, bool) {
	if localPath == "" {
		return "", false
	}
	ts := time.Now().Unix()
	return fmt.Sprintf("%s.bak-%d", localPath, ts), true
}
