package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/sam0uly/spin/internal/log"
	"github.com/sam0uly/spin/internal/registry"
)

// maybeBootstrapOfficial registers the official registry on first run
// and reports it. Once registries.json exists it is a permanent no-op,
// even if the user removed the official registry deliberately. A
// failure (e.g. no network) becomes a warning and the command proceeds.
func maybeBootstrapOfficial(ctx context.Context, mgr *registry.Manager) {
	did, err := mgr.Bootstrap(ctx)
	switch {
	case err != nil:
		log.Debug("could not bootstrap official registry", "err", err)
		printWarn("could not set up the official registry; run this command again to retry")
	case did:
		printInfo("bootstrapped official registry")
	}
}

// annotateShorthandError adds a re-add hint when shorthand resolution
// failed because the official registry is missing. Returns the
// enriched error and true, or the original error and false when no
// hint applies.
func annotateShorthandError(err error) (error, bool) {
	var notRegistered registry.AliasNotRegisteredError
	if errors.As(err, &notRegistered) && notRegistered.Alias == registry.OfficialAlias {
		return fmt.Errorf("%w (re-add it with: spin registry add %s %s)", err, registry.OfficialAlias, registry.DefaultRegistryURL), true
	}
	return err, false
}
