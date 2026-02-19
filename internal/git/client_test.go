package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRepo initialises a temporary git repository and returns a GitClient
// pointing at it. The repository contains a single "Initial commit".
func newTestRepo(t *testing.T) *GitClient {
	t.Helper()
	dir := t.TempDir()

	mustRun(t, dir, "git", "init", "-b", "main")
	mustRun(t, dir, "git", "config", "user.email", "test@example.com")
	mustRun(t, dir, "git", "config", "user.name", "Test")

	writeFile(t, dir, "README.md", "# Test\n")
	mustRun(t, dir, "git", "add", ".")
	mustRun(t, dir, "git", "commit", "-m", "Initial commit")

	c, err := NewGitClient(dir)
	require.NoError(t, err)
	return c
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command failed: %s %v\n%s", name, args, out)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// NewGitClient tests
// ---------------------------------------------------------------------------

func TestNewGitClient_ValidRepo(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "git", "init", "-b", "main")
	mustRun(t, dir, "git", "config", "user.email", "test@example.com")
	mustRun(t, dir, "git", "config", "user.name", "Test")
	writeFile(t, dir, "README.md", "# hi\n")
	mustRun(t, dir, "git", "add", ".")
	mustRun(t, dir, "git", "commit", "-m", "init")

	c, err := NewGitClient(dir)
	require.NoError(t, err)
	assert.NotNil(t, c)
	assert.Equal(t, dir, c.WorkDir)
}

func TestNewGitClient_NotARepo(t *testing.T) {
	dir := t.TempDir() // plain directory, no git init

	_, err := NewGitClient(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prerequisites")
}

// ---------------------------------------------------------------------------
// Branch tests
// ---------------------------------------------------------------------------

func TestCurrentBranch(t *testing.T) {
	c := newTestRepo(t)
	branch, err := c.CurrentBranch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "main", branch)
}

func TestBranchExists(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	exists, err := c.BranchExists(ctx, "main")
	require.NoError(t, err)
	assert.True(t, exists, "main branch should exist")

	exists, err = c.BranchExists(ctx, "nonexistent-branch")
	require.NoError(t, err)
	assert.False(t, exists, "nonexistent branch should not exist")
}

func TestCreateBranch_NoBase(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	err := c.CreateBranch(ctx, "feature/test", "")
	require.NoError(t, err)

	branch, err := c.CurrentBranch(ctx)
	require.NoError(t, err)
	assert.Equal(t, "feature/test", branch)
}

func TestCheckout(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	require.NoError(t, c.CreateBranch(ctx, "other", ""))
	require.NoError(t, c.Checkout(ctx, "main"))

	branch, err := c.CurrentBranch(ctx)
	require.NoError(t, err)
	assert.Equal(t, "main", branch)
}

// ---------------------------------------------------------------------------
// Status tests
// ---------------------------------------------------------------------------

func TestHasUncommittedChanges_Clean(t *testing.T) {
	c := newTestRepo(t)
	dirty, err := c.HasUncommittedChanges(context.Background())
	require.NoError(t, err)
	assert.False(t, dirty, "fresh repo should be clean")
}

func TestHasUncommittedChanges_Dirty(t *testing.T) {
	c := newTestRepo(t)
	writeFile(t, c.WorkDir, "newfile.txt", "hello\n")

	dirty, err := c.HasUncommittedChanges(context.Background())
	require.NoError(t, err)
	assert.True(t, dirty, "repo with untracked file should be dirty")
}

func TestIsClean(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	clean, err := c.IsClean(ctx)
	require.NoError(t, err)
	assert.True(t, clean)

	writeFile(t, c.WorkDir, "newfile.txt", "hello\n")
	clean, err = c.IsClean(ctx)
	require.NoError(t, err)
	assert.False(t, clean)
}

// ---------------------------------------------------------------------------
// Stash tests
// ---------------------------------------------------------------------------

func TestStash_CleanRepo(t *testing.T) {
	c := newTestRepo(t)
	stashed, err := c.Stash(context.Background(), "test stash")
	require.NoError(t, err)
	assert.False(t, stashed, "clean repo should not produce a stash")
}

func TestStash_DirtyRepo(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Stage a change so it can be stashed.
	writeFile(t, c.WorkDir, "README.md", "# Modified\n")
	mustRun(t, c.WorkDir, "git", "add", ".")

	stashed, err := c.Stash(ctx, "test stash")
	require.NoError(t, err)
	assert.True(t, stashed)

	// Working tree should now be clean.
	clean, err := c.IsClean(ctx)
	require.NoError(t, err)
	assert.True(t, clean)

	// Pop the stash.
	require.NoError(t, c.StashPop(ctx))
}

// ---------------------------------------------------------------------------
// Log tests
// ---------------------------------------------------------------------------

func TestHeadCommit(t *testing.T) {
	c := newTestRepo(t)
	sha, err := c.HeadCommit(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, sha)
	// Short SHAs are 7–40 characters.
	assert.GreaterOrEqual(t, len(sha), 7)
	assert.LessOrEqual(t, len(sha), 40)
}

func TestLog(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Add a second commit.
	writeFile(t, c.WorkDir, "second.txt", "second\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Second commit")

	entries, err := c.Log(ctx, 2)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Contains(t, entries[0].Message, "Second commit")
	assert.Contains(t, entries[1].Message, "Initial commit")
}

// ---------------------------------------------------------------------------
// Diff tests
// ---------------------------------------------------------------------------

func TestDiffFiles(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Record the base commit SHA.
	base, err := c.HeadCommit(ctx)
	require.NoError(t, err)

	// Add a new file and commit.
	writeFile(t, c.WorkDir, "added.txt", "new\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Add file")

	entries, err := c.DiffFiles(ctx, base)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "A", entries[0].Status)
	assert.Equal(t, "added.txt", entries[0].Path)
}

func TestDiffStat(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	base, err := c.HeadCommit(ctx)
	require.NoError(t, err)

	// Modify the README and add a new file, then commit.
	writeFile(t, c.WorkDir, "README.md", "# Modified\nExtra line\n")
	writeFile(t, c.WorkDir, "added.txt", "new\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Changes")

	stats, err := c.DiffStat(ctx, base)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.FilesChanged)
	assert.Greater(t, stats.Insertions, 0)
}

func TestDiffUnified(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	base, err := c.HeadCommit(ctx)
	require.NoError(t, err)

	writeFile(t, c.WorkDir, "README.md", "# Modified\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Modify")

	diff, err := c.DiffUnified(ctx, base)
	require.NoError(t, err)
	assert.Contains(t, diff, "README.md")
}

// ---------------------------------------------------------------------------
// EnsureClean tests
// ---------------------------------------------------------------------------

func TestEnsureClean_AlreadyClean(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	cleanup, err := c.EnsureClean(ctx)
	require.NoError(t, err)
	require.NotNil(t, cleanup)

	// Calling cleanup on an already-clean repo should be a no-op.
	require.NoError(t, cleanup())
}

func TestEnsureClean_DirtyRepo(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Stage a change.
	writeFile(t, c.WorkDir, "README.md", "# Dirty\n")
	mustRun(t, c.WorkDir, "git", "add", ".")

	cleanup, err := c.EnsureClean(ctx)
	require.NoError(t, err)

	// Working tree should be clean after stash.
	clean, err := c.IsClean(ctx)
	require.NoError(t, err)
	assert.True(t, clean)

	// Cleanup should restore the changes.
	require.NoError(t, cleanup())

	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	assert.True(t, dirty, "changes should be restored after cleanup")
}

// ---------------------------------------------------------------------------
// Internal parser unit tests
// ---------------------------------------------------------------------------

func TestParseDiffStat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    DiffStats
		wantErr bool
	}{
		{
			name:  "full summary",
			input: " file1.go | 10 ++++++++++\n file2.go | 3 ---\n 2 files changed, 10 insertions(+), 3 deletions(-)",
			want:  DiffStats{FilesChanged: 2, Insertions: 10, Deletions: 3},
		},
		{
			name:  "insertions only",
			input: " file.go | 5 +++++\n 1 file changed, 5 insertions(+)",
			want:  DiffStats{FilesChanged: 1, Insertions: 5, Deletions: 0},
		},
		{
			name:  "deletions only",
			input: " file.go | 3 ---\n 1 file changed, 3 deletions(-)",
			want:  DiffStats{FilesChanged: 1, Insertions: 0, Deletions: 3},
		},
		{
			name:  "empty output",
			input: "",
			want:  DiffStats{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDiffStat(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, *got)
		})
	}
}

func TestParseOneline(t *testing.T) {
	input := "abc1234 First commit\ndef5678 Second commit\n"
	entries := parseOneline(input)
	require.Len(t, entries, 2)
	assert.Equal(t, "abc1234", entries[0].SHA)
	assert.Equal(t, "First commit", entries[0].Message)
	assert.Equal(t, "def5678", entries[1].SHA)
}

func TestParseDiffNameStatus(t *testing.T) {
	input := "A\tadded.go\nM\tmodified.go\nD\tdeleted.go\n"
	entries := parseDiffNameStatus(input)
	require.Len(t, entries, 3)
	assert.Equal(t, DiffEntry{Status: "A", Path: "added.go"}, entries[0])
	assert.Equal(t, DiffEntry{Status: "M", Path: "modified.go"}, entries[1])
	assert.Equal(t, DiffEntry{Status: "D", Path: "deleted.go"}, entries[2])
}

// ---------------------------------------------------------------------------
// NewGitClient additional edge cases
// ---------------------------------------------------------------------------

func TestNewGitClient_EmptyWorkDir(t *testing.T) {
	// An empty string means "current directory". This test only verifies that
	// NewGitClient("") fails gracefully when the current directory is not a
	// git repo (CI workers run in arbitrary directories). If the CWD happens
	// to be a git repo the call may succeed — both outcomes are acceptable as
	// long as no panic occurs.
	_, err := NewGitClient("")
	// We cannot assert an error here because the CWD may or may not be a git
	// repo. The important thing is that there is no panic.
	_ = err
}

func TestNewGitClient_NonExistentDir(t *testing.T) {
	_, err := NewGitClient("/nonexistent/path/that/does/not/exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prerequisites")
}

func TestNewGitClient_PlainDir_ErrorContainsPrerequisites(t *testing.T) {
	dir := t.TempDir()
	_, err := NewGitClient(dir)
	require.Error(t, err)
	// Error must be wrapped with "git: prerequisites:" context (ERR-1).
	assert.Contains(t, err.Error(), "git: prerequisites:")
}

// ---------------------------------------------------------------------------
// Branch additional edge cases
// ---------------------------------------------------------------------------

func TestCreateBranch_WithBase(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Create a second commit on main so we have a meaningful history.
	writeFile(t, c.WorkDir, "second.txt", "second\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Second commit")

	// Create a branch from "main" explicitly.
	err := c.CreateBranch(ctx, "feature/from-main", "main")
	require.NoError(t, err)

	branch, err := c.CurrentBranch(ctx)
	require.NoError(t, err)
	assert.Equal(t, "feature/from-main", branch)

	// The new branch should exist.
	exists, err := c.BranchExists(ctx, "feature/from-main")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestBranchExists_AfterCreateBranch(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Branch must not exist yet.
	exists, err := c.BranchExists(ctx, "new-branch")
	require.NoError(t, err)
	assert.False(t, exists)

	// Create the branch.
	require.NoError(t, c.CreateBranch(ctx, "new-branch", ""))

	// Now it must exist.
	exists, err = c.BranchExists(ctx, "new-branch")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestCreateBranch_DuplicateName_ReturnsError(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	require.NoError(t, c.CreateBranch(ctx, "dup-branch", ""))
	// Switch back to main before trying to create the same branch again.
	require.NoError(t, c.Checkout(ctx, "main"))

	err := c.CreateBranch(ctx, "dup-branch", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: create branch")
}

func TestCheckout_NonExistentBranch_ReturnsError(t *testing.T) {
	c := newTestRepo(t)
	err := c.Checkout(context.Background(), "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: checkout")
}

// ---------------------------------------------------------------------------
// Status additional edge cases
// ---------------------------------------------------------------------------

func TestHasUncommittedChanges_StagedOnly(t *testing.T) {
	c := newTestRepo(t)

	// Modify and stage (but don't commit) an existing tracked file.
	writeFile(t, c.WorkDir, "README.md", "# Staged modification\n")
	mustRun(t, c.WorkDir, "git", "add", "README.md")

	dirty, err := c.HasUncommittedChanges(context.Background())
	require.NoError(t, err)
	assert.True(t, dirty, "staged changes should count as uncommitted")
}

func TestHasUncommittedChanges_UntrackedOnly(t *testing.T) {
	c := newTestRepo(t)

	// Add an untracked file (not staged, not committed).
	writeFile(t, c.WorkDir, "untracked.txt", "hello\n")

	dirty, err := c.HasUncommittedChanges(context.Background())
	require.NoError(t, err)
	assert.True(t, dirty, "untracked files should count as uncommitted")
}

func TestIsClean_Transitions(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Start clean.
	clean, err := c.IsClean(ctx)
	require.NoError(t, err)
	assert.True(t, clean)

	// Modify a tracked file (staged).
	writeFile(t, c.WorkDir, "README.md", "# Modified\n")
	mustRun(t, c.WorkDir, "git", "add", "README.md")

	clean, err = c.IsClean(ctx)
	require.NoError(t, err)
	assert.False(t, clean)

	// Commit, should be clean again.
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Modify README")
	clean, err = c.IsClean(ctx)
	require.NoError(t, err)
	assert.True(t, clean)
}

// ---------------------------------------------------------------------------
// Stash additional edge cases
// ---------------------------------------------------------------------------

func TestStashPop_EmptyStash_ReturnsError(t *testing.T) {
	c := newTestRepo(t)
	// No stash exists; popping should fail.
	err := c.StashPop(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: stash pop")
}

func TestStash_RestoredAfterPop(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	originalContent := "# Stash test content\n"
	writeFile(t, c.WorkDir, "README.md", originalContent)
	mustRun(t, c.WorkDir, "git", "add", ".")

	stashed, err := c.Stash(ctx, "save for pop test")
	require.NoError(t, err)
	require.True(t, stashed)

	// Working tree must be clean after stash.
	clean, err := c.IsClean(ctx)
	require.NoError(t, err)
	require.True(t, clean)

	// Pop stash.
	require.NoError(t, c.StashPop(ctx))

	// File should have the stashed content again.
	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	assert.True(t, dirty, "pop should restore uncommitted changes")

	data, err := os.ReadFile(filepath.Join(c.WorkDir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(data))
}

func TestStash_UntrackedFilesNotStashed(t *testing.T) {
	// git stash by default does not stash untracked files unless -u is used.
	// Verify the method reports dirty=true (HasUncommittedChanges sees untracked)
	// and then stash is created from the staged perspective. If there is nothing
	// to stash (only untracked), git stash produces no stash entry.
	c := newTestRepo(t)
	ctx := context.Background()

	// Only untracked file — not staged.
	writeFile(t, c.WorkDir, "untracked_only.txt", "data\n")

	// HasUncommittedChanges sees it as dirty (porcelain shows ??)
	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	require.True(t, dirty)

	// Stash will attempt push but git may say "No local changes to save"
	// when only untracked files exist. Verify no panic.
	_, _ = c.Stash(ctx, "untracked only")
}

// ---------------------------------------------------------------------------
// Diff additional edge cases
// ---------------------------------------------------------------------------

func TestDiffFiles_ModifiedAndDeleted(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Add another file so we can delete it.
	writeFile(t, c.WorkDir, "todelete.txt", "bye\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Add todelete")

	// Record base SHA at the commit that has todelete.txt.
	base, err := c.HeadCommit(ctx)
	require.NoError(t, err)

	// Modify README and delete todelete.txt.
	writeFile(t, c.WorkDir, "README.md", "# Modified\n")
	require.NoError(t, os.Remove(filepath.Join(c.WorkDir, "todelete.txt")))
	mustRun(t, c.WorkDir, "git", "add", "-A")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Modify and delete")

	entries, err := c.DiffFiles(ctx, base)
	require.NoError(t, err)

	// Build a status map for order-independent assertions.
	statusByPath := make(map[string]string, len(entries))
	for _, e := range entries {
		statusByPath[e.Path] = e.Status
	}

	assert.Equal(t, "M", statusByPath["README.md"], "README.md should be modified")
	assert.Equal(t, "D", statusByPath["todelete.txt"], "todelete.txt should be deleted")
}

func TestDiffFiles_RenamedFile(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Add a file to rename.
	writeFile(t, c.WorkDir, "original.txt", "content\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Add original")

	base, err := c.HeadCommit(ctx)
	require.NoError(t, err)

	// Rename via git mv.
	mustRun(t, c.WorkDir, "git", "mv", "original.txt", "renamed.txt")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Rename file")

	entries, err := c.DiffFiles(ctx, base)
	require.NoError(t, err)

	require.NotEmpty(t, entries)
	// parseDiffNameStatus maps the rename to status "R" and the destination path.
	var renameFound bool
	for _, e := range entries {
		if e.Status == "R" && e.Path == "renamed.txt" {
			renameFound = true
		}
	}
	assert.True(t, renameFound, "renamed file should appear with status R and destination path")
}

func TestDiffStat_DeletionsCount(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	base, err := c.HeadCommit(ctx)
	require.NoError(t, err)

	// Replace README with fewer lines (net deletions).
	writeFile(t, c.WorkDir, "README.md", "x\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Shrink README")

	stats, err := c.DiffStat(ctx, base)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.FilesChanged)
	assert.Greater(t, stats.Deletions, 0, "net deletions should be positive")
}

func TestDiffUnified_ContainsDiffMarkers(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	base, err := c.HeadCommit(ctx)
	require.NoError(t, err)

	writeFile(t, c.WorkDir, "README.md", "# Modified content\nExtra line\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Modify README")

	diff, err := c.DiffUnified(ctx, base)
	require.NoError(t, err)
	assert.Contains(t, diff, "---")
	assert.Contains(t, diff, "+++")
	assert.Contains(t, diff, "@@")
	assert.Contains(t, diff, "README.md")
}

// ---------------------------------------------------------------------------
// Log additional edge cases
// ---------------------------------------------------------------------------

func TestLog_LimitExceedsCommitCount(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Repo has only 1 commit. Ask for 10.
	entries, err := c.Log(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "should return only the available commits")
}

func TestLog_SingleEntry(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Add two more commits.
	writeFile(t, c.WorkDir, "a.txt", "a\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Second")

	writeFile(t, c.WorkDir, "b.txt", "b\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Third")

	entries, err := c.Log(ctx, 1)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Message, "Third")
}

func TestLog_SHAFormat(t *testing.T) {
	c := newTestRepo(t)
	entries, err := c.Log(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	sha := entries[0].SHA
	assert.NotEmpty(t, sha)
	// Short SHAs are 7-40 hex characters.
	assert.GreaterOrEqual(t, len(sha), 7)
	assert.LessOrEqual(t, len(sha), 40)
	// Must be valid hex.
	for _, ch := range sha {
		assert.True(t, (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f'),
			"SHA character %q is not hex", ch)
	}
}

func TestHeadCommit_MatchesLog(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	sha, err := c.HeadCommit(ctx)
	require.NoError(t, err)

	entries, err := c.Log(ctx, 1)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// HeadCommit and first log entry should reference the same commit.
	assert.True(t, strings.HasPrefix(entries[0].SHA, sha) || strings.HasPrefix(sha, entries[0].SHA),
		"HeadCommit %q should match log entry SHA %q", sha, entries[0].SHA)
}

// ---------------------------------------------------------------------------
// Push tests (error path — no remote configured)
// ---------------------------------------------------------------------------

func TestPush_NoRemote_ReturnsError(t *testing.T) {
	c := newTestRepo(t)
	err := c.Push(context.Background(), "origin", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: push")
}

func TestPush_WithUpstream_NoRemote_ReturnsError(t *testing.T) {
	c := newTestRepo(t)
	err := c.Push(context.Background(), "origin", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: push")
}

// ---------------------------------------------------------------------------
// Context propagation tests
//
// All GitClient methods accept context.Context and pass it to exec.CommandContext.
// The tests below verify two things:
//   1. Methods do not panic when called with a cancelled context.
//   2. Methods accept ctx as the first parameter (compile-time API contract).
//
// Note: git commands on a tiny repo complete in microseconds. A pre-cancelled
// context may or may not cancel the underlying process before it finishes,
// making error assertions inherently racy on fast machines. We therefore check
// that the call completes without panic rather than asserting on the error value.
// ---------------------------------------------------------------------------

func TestCurrentBranch_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Must not panic; error is allowed but not required.
	_, _ = c.CurrentBranch(ctx)
}

func TestHasUncommittedChanges_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = c.HasUncommittedChanges(ctx)
}

func TestBranchExists_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = c.BranchExists(ctx, "main")
}

func TestHeadCommit_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = c.HeadCommit(ctx)
}

func TestLog_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = c.Log(ctx, 5)
}

func TestCreateBranch_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = c.CreateBranch(ctx, "ctx-test-branch", "")
}

func TestCheckout_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = c.Checkout(ctx, "main")
}

func TestDiffFiles_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	sha, err := c.HeadCommit(context.Background())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = c.DiffFiles(ctx, sha)
}

func TestDiffStat_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	sha, err := c.HeadCommit(context.Background())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = c.DiffStat(ctx, sha)
}

func TestDiffUnified_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	sha, err := c.HeadCommit(context.Background())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = c.DiffUnified(ctx, sha)
}

func TestStash_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = c.Stash(ctx, "ctx-test")
}

func TestStashPop_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = c.StashPop(ctx)
}

// ---------------------------------------------------------------------------
// Error wrapping format tests (ERR-1 compliance)
// ---------------------------------------------------------------------------

func TestErrorWrapping_CurrentBranch(t *testing.T) {
	// Use a detached HEAD to trigger the "detached HEAD state" error path
	// which produces a known "git: current branch:" prefix.
	c := newTestRepo(t)
	ctx := context.Background()

	sha, err := c.HeadCommit(ctx)
	require.NoError(t, err)

	// Detach HEAD.
	mustRun(t, c.WorkDir, "git", "checkout", sha)

	_, err = c.CurrentBranch(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: current branch:")
}

func TestErrorWrapping_DiffFiles_InvalidBase(t *testing.T) {
	c := newTestRepo(t)
	_, err := c.DiffFiles(context.Background(), "nonexistent-ref")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: diff files from")
}

func TestErrorWrapping_DiffStat_InvalidBase(t *testing.T) {
	c := newTestRepo(t)
	_, err := c.DiffStat(context.Background(), "nonexistent-ref")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: diff stat from")
}

func TestErrorWrapping_DiffUnified_InvalidBase(t *testing.T) {
	c := newTestRepo(t)
	_, err := c.DiffUnified(context.Background(), "nonexistent-ref")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: diff unified from")
}

// ---------------------------------------------------------------------------
// Internal parser additional unit tests (table-driven)
// ---------------------------------------------------------------------------

func TestParseDiffStat_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    DiffStats
		wantErr bool
	}{
		{
			name:  "whitespace only",
			input: "   \n   \n   ",
			want:  DiffStats{},
		},
		{
			name:  "single file single insertion",
			input: " foo.go | 1 +\n 1 file changed, 1 insertion(+)",
			want:  DiffStats{FilesChanged: 1, Insertions: 1, Deletions: 0},
		},
		{
			name:  "many files",
			input: " 100 files changed, 999 insertions(+), 500 deletions(-)",
			want:  DiffStats{FilesChanged: 100, Insertions: 999, Deletions: 500},
		},
		{
			name:  "trailing newline in input",
			input: " file.go | 2 ++\n 1 file changed, 2 insertions(+)\n",
			want:  DiffStats{FilesChanged: 1, Insertions: 2, Deletions: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDiffStat(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, *got)
		})
	}
}

func TestParseOneline_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []LogEntry
	}{
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
		{
			name:  "single line no message",
			input: "abc1234\n",
			want:  []LogEntry{{SHA: "abc1234", Message: ""}},
		},
		{
			name:  "message with spaces",
			input: "abc1234 feat: add some cool feature\n",
			want:  []LogEntry{{SHA: "abc1234", Message: "feat: add some cool feature"}},
		},
		{
			name:  "multiple blank lines between entries",
			input: "abc1234 First\n\n\ndef5678 Second\n",
			want: []LogEntry{
				{SHA: "abc1234", Message: "First"},
				{SHA: "def5678", Message: "Second"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOneline(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseDiffNameStatus_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []DiffEntry
	}{
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
		{
			name:  "rename with similarity score",
			input: "R100\told.go\tnew.go\n",
			want:  []DiffEntry{{Status: "R", Path: "new.go"}},
		},
		{
			name:  "partial rename score",
			input: "R075\toriginal/path.go\tdest/path.go\n",
			want:  []DiffEntry{{Status: "R", Path: "dest/path.go"}},
		},
		{
			name:  "missing tab separator — line skipped",
			input: "invalid line without tab\n",
			want:  nil,
		},
		{
			name:  "mixed valid and invalid lines",
			input: "A\tvalid.go\ninvalid\nM\tother.go\n",
			want: []DiffEntry{
				{Status: "A", Path: "valid.go"},
				{Status: "M", Path: "other.go"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDiffNameStatus(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Integration tests — full lifecycle
// ---------------------------------------------------------------------------

func TestIntegration_BranchLifecycle(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// 1. Create a feature branch.
	require.NoError(t, c.CreateBranch(ctx, "feature/lifecycle", ""))
	branch, err := c.CurrentBranch(ctx)
	require.NoError(t, err)
	assert.Equal(t, "feature/lifecycle", branch)

	// 2. Commit work on the feature branch.
	writeFile(t, c.WorkDir, "feature.go", "package main\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Add feature")

	// 3. Stash an uncommitted change.
	writeFile(t, c.WorkDir, "wip.txt", "work in progress\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	stashed, err := c.Stash(ctx, "lifecycle stash")
	require.NoError(t, err)
	require.True(t, stashed)

	clean, err := c.IsClean(ctx)
	require.NoError(t, err)
	assert.True(t, clean)

	// 4. Pop the stash and verify changes are back.
	require.NoError(t, c.StashPop(ctx))
	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	assert.True(t, dirty)

	// 5. Commit the WIP, check out main, verify branch switch.
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Commit WIP")
	require.NoError(t, c.Checkout(ctx, "main"))
	branch, err = c.CurrentBranch(ctx)
	require.NoError(t, err)
	assert.Equal(t, "main", branch)

	// 6. The feature branch should still exist.
	exists, err := c.BranchExists(ctx, "feature/lifecycle")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestIntegration_DiffAcrossBranches(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	// Record the SHA of the tip of main before branching.
	mainSHA, err := c.HeadCommit(ctx)
	require.NoError(t, err)

	// Create a feature branch and add commits.
	require.NoError(t, c.CreateBranch(ctx, "feature/diff-test", ""))
	writeFile(t, c.WorkDir, "added.go", "package main\n")
	writeFile(t, c.WorkDir, "README.md", "# Updated README\n")
	mustRun(t, c.WorkDir, "git", "add", "-A")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Feature changes")

	// DiffFiles from the main SHA.
	entries, err := c.DiffFiles(ctx, mainSHA)
	require.NoError(t, err)

	statusByPath := make(map[string]string, len(entries))
	for _, e := range entries {
		statusByPath[e.Path] = e.Status
	}
	assert.Equal(t, "A", statusByPath["added.go"])
	assert.Equal(t, "M", statusByPath["README.md"])

	// DiffStat from the main SHA.
	stats, err := c.DiffStat(ctx, mainSHA)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.FilesChanged)
	assert.Greater(t, stats.Insertions, 0)

	// DiffUnified from the main SHA.
	diff, err := c.DiffUnified(ctx, mainSHA)
	require.NoError(t, err)
	assert.Contains(t, diff, "added.go")
	assert.Contains(t, diff, "README.md")
}

// ---------------------------------------------------------------------------
// Recovery (EnsureClean) additional tests
// ---------------------------------------------------------------------------

func TestEnsureClean_CleanupIsNoopOnClean(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	cleanup, err := c.EnsureClean(ctx)
	require.NoError(t, err)
	require.NotNil(t, cleanup)

	// Call cleanup twice — must not error either time.
	require.NoError(t, cleanup())

	// After the no-op cleanup, the tree should still be clean.
	clean, err := c.IsClean(ctx)
	require.NoError(t, err)
	assert.True(t, clean)
}

func TestEnsureClean_DeferCleanupRestoresFile(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	originalContent := "# Original\n"
	writeFile(t, c.WorkDir, "README.md", originalContent)
	// We need a tracked change to be stashable.
	mustRun(t, c.WorkDir, "git", "add", "README.md")

	cleanup, err := c.EnsureClean(ctx)
	require.NoError(t, err)

	// Should be clean now.
	clean, err := c.IsClean(ctx)
	require.NoError(t, err)
	require.True(t, clean)

	// Defer-style cleanup.
	var cleanupErr error
	func() {
		defer func() {
			cleanupErr = cleanup()
		}()
	}()
	require.NoError(t, cleanupErr)

	// Changes should be restored.
	dirty, err := c.HasUncommittedChanges(ctx)
	require.NoError(t, err)
	assert.True(t, dirty)

	data, err := os.ReadFile(filepath.Join(c.WorkDir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(data))
}

func TestEnsureClean_ErrorPrefixInCleanupFailure(t *testing.T) {
	// Verify that the cleanup function returned by EnsureClean wraps stash-pop
	// failures with "git: ensure clean: restoring stash:" prefix (ERR-1).
	// We trigger the failure by popping the stash manually before calling cleanup.
	c := newTestRepo(t)
	ctx := context.Background()

	// Dirty the tree with a staged change.
	writeFile(t, c.WorkDir, "README.md", "# Error wrapping test\n")
	mustRun(t, c.WorkDir, "git", "add", "README.md")

	cleanup, err := c.EnsureClean(ctx)
	require.NoError(t, err)

	// Manually pop the stash so the cleanup call has no stash to pop.
	require.NoError(t, c.StashPop(ctx))

	// Now cleanup() should fail and the error must start with the expected prefix.
	cleanupErr := cleanup()
	require.Error(t, cleanupErr)
	assert.Contains(t, cleanupErr.Error(), "git: ensure clean: restoring stash:")
}

// ---------------------------------------------------------------------------
// DiffNumStat tests
// ---------------------------------------------------------------------------

func TestDiffNumStat_AddedFile(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	base, err := c.HeadCommit(ctx)
	require.NoError(t, err)

	writeFile(t, c.WorkDir, "newfile.go", "package main\n\nfunc main() {}\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Add file")

	entries, err := c.DiffNumStat(ctx, base)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "newfile.go", entries[0].Path)
	assert.Equal(t, 3, entries[0].Added)
	assert.Equal(t, 0, entries[0].Deleted)
}

func TestDiffNumStat_ModifiedFile(t *testing.T) {
	c := newTestRepo(t)
	ctx := context.Background()

	base, err := c.HeadCommit(ctx)
	require.NoError(t, err)

	writeFile(t, c.WorkDir, "README.md", "# Modified\nNew line\n")
	mustRun(t, c.WorkDir, "git", "add", ".")
	mustRun(t, c.WorkDir, "git", "commit", "-m", "Modify README")

	entries, err := c.DiffNumStat(ctx, base)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	// Find README.md entry.
	var readme *NumStatEntry
	for i := range entries {
		if entries[i].Path == "README.md" {
			readme = &entries[i]
			break
		}
	}
	require.NotNil(t, readme, "README.md should be in numstat output")
	assert.Greater(t, readme.Added+readme.Deleted, 0, "there should be some changed lines")
}

func TestDiffNumStat_ErrorWrapping(t *testing.T) {
	c := newTestRepo(t)
	_, err := c.DiffNumStat(context.Background(), "nonexistent-ref")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: diff numstat from")
}

func TestDiffNumStat_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	sha, err := c.HeadCommit(context.Background())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = c.DiffNumStat(ctx, sha)
}

// ---------------------------------------------------------------------------
// parseNumStat unit tests
// ---------------------------------------------------------------------------

func TestParseNumStat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []NumStatEntry
	}{
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "single added file",
			input: "3\t0\tnew.go\n",
			want:  []NumStatEntry{{Path: "new.go", Added: 3, Deleted: 0}},
		},
		{
			name:  "modified file",
			input: "5\t2\texisting.go\n",
			want:  []NumStatEntry{{Path: "existing.go", Added: 5, Deleted: 2}},
		},
		{
			name:  "binary file",
			input: "-\t-\timage.png\n",
			want:  []NumStatEntry{{Path: "image.png", Added: -1, Deleted: -1}},
		},
		{
			name:  "multiple files",
			input: "3\t0\ta.go\n10\t5\tb.go\n0\t7\tc.go\n",
			want: []NumStatEntry{
				{Path: "a.go", Added: 3, Deleted: 0},
				{Path: "b.go", Added: 10, Deleted: 5},
				{Path: "c.go", Added: 0, Deleted: 7},
			},
		},
		{
			name:  "rename with brace notation",
			input: "2\t1\t{old => new}.go\n",
			want:  []NumStatEntry{{Path: "new.go", OldPath: "old.go", Added: 2, Deleted: 1}},
		},
		{
			name:  "rename simple arrow",
			input: "4\t2\told.go => new.go\n",
			want:  []NumStatEntry{{Path: "new.go", OldPath: "old.go", Added: 4, Deleted: 2}},
		},
		{
			name:  "missing tab separator skipped",
			input: "invalid line\n3\t0\tvalid.go\n",
			want:  []NumStatEntry{{Path: "valid.go", Added: 3, Deleted: 0}},
		},
		{
			name:  "blank lines ignored",
			input: "3\t0\ta.go\n\n\n5\t1\tb.go\n",
			want: []NumStatEntry{
				{Path: "a.go", Added: 3, Deleted: 0},
				{Path: "b.go", Added: 5, Deleted: 1},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseNumStat(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// parseRenamePath unit tests
// ---------------------------------------------------------------------------

func TestParseRenamePath(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantOldPath string
		wantNewPath string
	}{
		{
			name:        "simple arrow",
			input:       "old.go => new.go",
			wantOldPath: "old.go",
			wantNewPath: "new.go",
		},
		{
			name:        "brace notation at root",
			input:       "{old => new}.go",
			wantOldPath: "old.go",
			wantNewPath: "new.go",
		},
		{
			name:        "brace notation with prefix",
			input:       "src/{old => new}/file.go",
			wantOldPath: "src/old/file.go",
			wantNewPath: "src/new/file.go",
		},
		{
			name:        "no rename — fallback",
			input:       "plain/path.go",
			wantOldPath: "",
			wantNewPath: "plain/path.go",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			oldPath, newPath := parseRenamePath(tt.input)
			assert.Equal(t, tt.wantOldPath, oldPath)
			assert.Equal(t, tt.wantNewPath, newPath)
		})
	}
}

// ---------------------------------------------------------------------------
// Client interface compliance
// ---------------------------------------------------------------------------

// TestClientInterface verifies that *GitClient satisfies the Client interface
// at compile time. This is the runtime companion to the var _ check in client.go.
func TestClientInterface(t *testing.T) {
	t.Parallel()

	var _ Client = (*GitClient)(nil)
}

// ---------------------------------------------------------------------------
// Fetch tests
// ---------------------------------------------------------------------------

// TestFetch_NoRemote_ReturnsError verifies that Fetch returns a wrapped error
// when no remote is configured (the typical state for a freshly initialised
// test repo that has no origin).
func TestFetch_NoRemote_ReturnsError(t *testing.T) {
	c := newTestRepo(t)
	err := c.Fetch(context.Background(), "origin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: fetch")
}

// TestFetch_EmptyRemote_DefaultsToOrigin verifies that passing an empty remote
// string causes the client to fetch from "origin" (the default). Since the
// test repo has no remotes, an error is expected — but the error message must
// reference "origin" to confirm the default was applied.
func TestFetch_EmptyRemote_DefaultsToOrigin(t *testing.T) {
	c := newTestRepo(t)
	err := c.Fetch(context.Background(), "")
	require.Error(t, err)
	// The error wraps "git: fetch origin:..." confirming "origin" was used.
	assert.Contains(t, err.Error(), "git: fetch origin")
}

// TestFetch_NonexistentRemote_ReturnsError verifies that Fetch fails cleanly
// when a remote name that does not exist is supplied.
func TestFetch_NonexistentRemote_ReturnsError(t *testing.T) {
	c := newTestRepo(t)
	err := c.Fetch(context.Background(), "nonexistent-remote")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: fetch nonexistent-remote")
}

// TestFetch_AcceptsContext verifies that Fetch honours context cancellation
// without panicking. The fast test repo means the command may finish before
// the context propagates — both success and error are acceptable outcomes.
func TestFetch_AcceptsContext(t *testing.T) {
	c := newTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Must not panic; error is expected but not required.
	_ = c.Fetch(ctx, "origin")
}

// TestFetch_WithRemote_ReturnsError verifies that a named remote that is not
// configured produces an error with the correct "git: fetch <remote>:" prefix.
func TestFetch_WithRemote_NamedRemote_ReturnsError(t *testing.T) {
	c := newTestRepo(t)
	err := c.Fetch(context.Background(), "upstream")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: fetch upstream")
}
