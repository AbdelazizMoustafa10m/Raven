package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// EnsureClean — recovery_test.go
//
// These tests focus on the EnsureClean helper defined in recovery.go.
// The helper creates an auto-stash when the working tree is dirty and returns
// a cleanup function that pops the stash when called.
// ---------------------------------------------------------------------------

// TestEnsureClean_CleanRepo verifies that EnsureClean on a clean working tree
// returns a no-op cleanup function (i.e., calling cleanup() has no effect and
// returns nil).
func TestEnsureClean_CleanRepo(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	cleanup, err := c.EnsureClean(ctx)
	require.NoError(t, err)
	require.NotNil(t, cleanup, "cleanup must never be nil")

	// Cleanup must be safe to call on a clean repo.
	require.NoError(t, cleanup(), "no-op cleanup should not return an error")

	// Tree should still be clean after cleanup.
	clean, err := c.IsClean(ctx)
	require.NoError(t, err)
	assert.True(t, clean, "working tree must remain clean after no-op cleanup")
}

// TestEnsureClean_DirtyRepo_CleanupPopsStash verifies that EnsureClean stashes
// dirty changes and the returned cleanup function restores them via stash pop.
func TestEnsureClean_DirtyRepo_CleanupPopsStash(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Dirty the working tree with a staged change (required for stash).
	modifiedContent := "# Dirty for recovery test\n"
	writeFile(t, c.WorkDir, "README.md", modifiedContent)
	mustRun(t, c.WorkDir, "git", "add", "README.md")

	// Confirm dirty before EnsureClean.
	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	require.True(t, dirty, "tree must be dirty before EnsureClean")

	cleanup, err := c.EnsureClean(ctx)
	require.NoError(t, err)
	require.NotNil(t, cleanup)

	// Working tree should be clean now.
	clean, err := c.IsClean(ctx)
	require.NoError(t, err)
	require.True(t, clean, "EnsureClean should have stashed changes")

	// Run cleanup (as one would via defer).
	require.NoError(t, cleanup(), "cleanup (stash pop) must succeed")

	// Changes should be restored.
	dirty, err = c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	assert.True(t, dirty, "cleanup must restore uncommitted changes")

	// The file content must match what we staged.
	data, err := os.ReadFile(filepath.Join(c.WorkDir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, modifiedContent, string(data), "file content must be restored")
}

// TestEnsureClean_MultipleCleanupCalls verifies the expectation around calling
// cleanup more than once. The first call pops the stash; the second call will
// fail because the stash is already empty. This is the documented behaviour —
// callers should call cleanup exactly once (typically via defer).
func TestEnsureClean_MultipleCleanupCalls_SecondFails(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Dirty the tree.
	writeFile(t, c.WorkDir, "README.md", "# Dirty\n")
	mustRun(t, c.WorkDir, "git", "add", "README.md")

	cleanup, err := c.EnsureClean(ctx)
	require.NoError(t, err)

	// First cleanup pops the stash — must succeed.
	require.NoError(t, cleanup())

	// Second cleanup tries to pop an empty stash — must fail.
	err = cleanup()
	require.Error(t, err, "second cleanup call should fail because stash is empty")
	assert.Contains(t, err.Error(), "git: ensure clean: restoring stash:")
}

// TestEnsureClean_CleanupErrorWrapping verifies the error message produced when
// the stash pop inside cleanup fails. The error must contain the documented
// "git: ensure clean: restoring stash:" prefix.
func TestEnsureClean_CleanupErrorWrapping(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Dirty the tree so EnsureClean stashes.
	writeFile(t, c.WorkDir, "README.md", "# Stash me\n")
	mustRun(t, c.WorkDir, "git", "add", "README.md")

	cleanup, err := c.EnsureClean(ctx)
	require.NoError(t, err)

	// Pop the stash manually so the cleanup function's pop will fail.
	require.NoError(t, c.StashPop(ctx))

	err = cleanup()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: ensure clean: restoring stash:")
}

// TestEnsureClean_WithNewFile_UntrackedNotStashed tests that EnsureClean
// correctly handles the case where the only dirty state is an untracked file.
// git stash (without -u) does NOT stash untracked files, so the tree may still
// appear dirty after the stash. The function should not error even in this case.
func TestEnsureClean_WithNewUntrackedFile(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Only an untracked file — not staged.
	writeFile(t, c.WorkDir, "untracked.txt", "hello\n")

	// HasUncommittedChanges sees untracked files as dirty.
	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	require.True(t, dirty)

	// EnsureClean calls Stash internally. git stash won't stash untracked-only
	// changes (returns "No local changes to save"), so Stash returns (false, nil).
	// EnsureClean should therefore return a no-op cleanup and no error.
	cleanup, err := c.EnsureClean(ctx)
	require.NoError(t, err)
	require.NotNil(t, cleanup)

	// The no-op cleanup should succeed.
	require.NoError(t, cleanup())
}

// TestEnsureClean_DeferPattern validates the idiomatic defer usage documented
// in the EnsureClean godoc comment.
func TestEnsureClean_DeferPattern(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Dirty the tree.
	writeFile(t, c.WorkDir, "README.md", "# Deferred stash test\n")
	mustRun(t, c.WorkDir, "git", "add", "README.md")

	var cleanupErr error

	func() {
		cleanup, err := c.EnsureClean(ctx)
		if err != nil {
			t.Fatalf("EnsureClean failed: %v", err)
		}
		defer func() {
			cleanupErr = cleanup()
		}()

		// Simulate work that requires a clean tree.
		clean, err := c.IsClean(ctx)
		require.NoError(t, err)
		assert.True(t, clean, "tree must be clean inside the operation")
	}()

	require.NoError(t, cleanupErr, "deferred cleanup must succeed")

	// After the deferred cleanup, changes must be restored.
	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	assert.True(t, dirty, "changes must be restored after deferred cleanup")
}
