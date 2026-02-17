package git

import (
	"context"
	"fmt"
)

// EnsureClean checks whether the working tree is clean. If dirty, it stashes
// the current changes and returns a cleanup function that pops the stash when
// called. If the tree is already clean, a no-op cleanup function is returned.
//
// The caller is responsible for always invoking the returned cleanup function,
// typically via defer:
//
//	cleanup, err := client.EnsureClean(ctx)
//	if err != nil {
//	    return err
//	}
//	defer func() {
//	    if cleanErr := cleanup(); cleanErr != nil {
//	        log.Error("stash pop failed", "err", cleanErr)
//	    }
//	}()
func (g *GitClient) EnsureClean(ctx context.Context) (cleanup func() error, err error) {
	stashed, err := g.Stash(ctx, "raven: auto-stash before operation")
	if err != nil {
		return nil, fmt.Errorf("git: ensure clean: %w", err)
	}

	if !stashed {
		// Working tree was already clean — nothing to undo.
		return func() error { return nil }, nil
	}

	// Return a cleanup function that pops the stash.
	return func() error {
		if popErr := g.StashPop(ctx); popErr != nil {
			return fmt.Errorf("git: ensure clean: restoring stash: %w", popErr)
		}
		return nil
	}, nil
}
