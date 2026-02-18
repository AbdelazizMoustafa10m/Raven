package review

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/git"
)

// ---------------------------------------------------------------------------
// Integration test helpers
// ---------------------------------------------------------------------------

// newIntegrationRepo initialises a temporary git repository with an initial
// commit on "main", returning a *git.GitClient pointing at it and the SHA
// of the initial commit so tests can diff from there.
func newIntegrationRepo(t *testing.T) (*git.GitClient, string) {
	t.Helper()
	dir := t.TempDir()

	integMustRun(t, dir, "git", "init", "-b", "main")
	integMustRun(t, dir, "git", "config", "user.email", "test@example.com")
	integMustRun(t, dir, "git", "config", "user.name", "Test")

	integWriteFile(t, dir, "README.md", "# Initial\n")
	integMustRun(t, dir, "git", "add", ".")
	integMustRun(t, dir, "git", "commit", "-m", "Initial commit")

	c, err := git.NewGitClient(dir)
	require.NoError(t, err)

	sha, err := c.HeadCommit(context.Background())
	require.NoError(t, err)

	return c, sha
}

func integMustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command failed: %s %v\n%s", name, args, out)
}

func integWriteFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// ---------------------------------------------------------------------------
// Integration — generate diff in a real git repository
// ---------------------------------------------------------------------------

func TestIntegration_Generate_AddedFile(t *testing.T) {
	gitClient, baseSHA := newIntegrationRepo(t)

	// Add a Go file and commit.
	integWriteFile(t, gitClient.WorkDir, "main.go", "package main\n\nfunc main() {}\n")
	integMustRun(t, gitClient.WorkDir, "git", "add", ".")
	integMustRun(t, gitClient.WorkDir, "git", "commit", "-m", "Add main.go")

	dg, err := NewDiffGenerator(gitClient, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), baseSHA)
	require.NoError(t, err)

	// main.go should appear as added.
	require.NotEmpty(t, result.Files)
	var found bool
	for _, f := range result.Files {
		if f.Path == "main.go" {
			found = true
			assert.Equal(t, ChangeAdded, f.ChangeType)
			assert.Greater(t, f.LinesAdded, 0)
			assert.Equal(t, 0, f.LinesDeleted)
		}
	}
	assert.True(t, found, "main.go should be in the diff result")
	assert.Equal(t, baseSHA, result.BaseBranch)
	assert.NotEmpty(t, result.FullDiff)
}

func TestIntegration_Generate_ModifiedFile(t *testing.T) {
	gitClient, baseSHA := newIntegrationRepo(t)

	// Modify the existing README.
	integWriteFile(t, gitClient.WorkDir, "README.md", "# Modified\nNew line added.\n")
	integMustRun(t, gitClient.WorkDir, "git", "add", ".")
	integMustRun(t, gitClient.WorkDir, "git", "commit", "-m", "Modify README")

	dg, err := NewDiffGenerator(gitClient, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), baseSHA)
	require.NoError(t, err)

	require.NotEmpty(t, result.Files)
	var readme *ChangedFile
	for i := range result.Files {
		if result.Files[i].Path == "README.md" {
			readme = &result.Files[i]
			break
		}
	}
	require.NotNil(t, readme, "README.md must be in the diff")
	assert.Equal(t, ChangeModified, readme.ChangeType)
	assert.Greater(t, readme.LinesAdded+readme.LinesDeleted, 0)
}

func TestIntegration_Generate_DeletedFile(t *testing.T) {
	gitClient, _ := newIntegrationRepo(t)
	ctx := context.Background()

	// Add a file then delete it across the diff boundary.
	integWriteFile(t, gitClient.WorkDir, "todelete.go", "package main\n")
	integMustRun(t, gitClient.WorkDir, "git", "add", ".")
	integMustRun(t, gitClient.WorkDir, "git", "commit", "-m", "Add todelete.go")

	baseSHA, err := gitClient.HeadCommit(ctx)
	require.NoError(t, err)

	require.NoError(t, os.Remove(filepath.Join(gitClient.WorkDir, "todelete.go")))
	integMustRun(t, gitClient.WorkDir, "git", "add", "-A")
	integMustRun(t, gitClient.WorkDir, "git", "commit", "-m", "Delete todelete.go")

	dg, err := NewDiffGenerator(gitClient, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(ctx, baseSHA)
	require.NoError(t, err)

	require.NotEmpty(t, result.Files)
	var found bool
	for _, f := range result.Files {
		if f.Path == "todelete.go" {
			found = true
			assert.Equal(t, ChangeDeleted, f.ChangeType)
		}
	}
	assert.True(t, found, "todelete.go should appear as deleted")
}

func TestIntegration_Generate_RenamedFile(t *testing.T) {
	gitClient, _ := newIntegrationRepo(t)
	ctx := context.Background()

	// Add a file then rename it.
	integWriteFile(t, gitClient.WorkDir, "original.go", "package main\n")
	integMustRun(t, gitClient.WorkDir, "git", "add", ".")
	integMustRun(t, gitClient.WorkDir, "git", "commit", "-m", "Add original.go")

	baseSHA, err := gitClient.HeadCommit(ctx)
	require.NoError(t, err)

	integMustRun(t, gitClient.WorkDir, "git", "mv", "original.go", "renamed.go")
	integMustRun(t, gitClient.WorkDir, "git", "commit", "-m", "Rename original.go to renamed.go")

	dg, err := NewDiffGenerator(gitClient, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(ctx, baseSHA)
	require.NoError(t, err)

	require.NotEmpty(t, result.Files)
	var found bool
	for _, f := range result.Files {
		if f.Path == "renamed.go" {
			found = true
			assert.Equal(t, ChangeRenamed, f.ChangeType)
		}
	}
	assert.True(t, found, "renamed.go should appear with ChangeRenamed type")
}

func TestIntegration_Generate_ExtensionFilterInRealRepo(t *testing.T) {
	gitClient, baseSHA := newIntegrationRepo(t)

	// Add both a .go file and a .md file.
	integWriteFile(t, gitClient.WorkDir, "handler.go", "package main\n\nfunc Handle() {}\n")
	integWriteFile(t, gitClient.WorkDir, "CHANGELOG.md", "## v1.0\n- initial release\n")
	integMustRun(t, gitClient.WorkDir, "git", "add", ".")
	integMustRun(t, gitClient.WorkDir, "git", "commit", "-m", "Add handler and changelog")

	// Only include .go files.
	dg, err := NewDiffGenerator(gitClient, ReviewConfig{Extensions: `\.go$`}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), baseSHA)
	require.NoError(t, err)

	for _, f := range result.Files {
		assert.True(t,
			strings.HasSuffix(f.Path, ".go"),
			"only .go files should be included, got %q", f.Path,
		)
	}

	// handler.go must be present.
	var goFound bool
	for _, f := range result.Files {
		if f.Path == "handler.go" {
			goFound = true
		}
	}
	assert.True(t, goFound, "handler.go must be included after extension filter")
}

func TestIntegration_Generate_RiskClassificationInRealRepo(t *testing.T) {
	gitClient, baseSHA := newIntegrationRepo(t)

	// Create an auth handler file (should be high-risk) and a utility file (normal).
	require.NoError(t, os.MkdirAll(
		filepath.Join(gitClient.WorkDir, "internal", "auth"), 0o755))
	integWriteFile(t, gitClient.WorkDir,
		filepath.Join("internal", "auth", "handler.go"),
		"package auth\n\nfunc HandleAuth() {}\n",
	)
	integWriteFile(t, gitClient.WorkDir, "util.go", "package main\n\nfunc Util() {}\n")
	integMustRun(t, gitClient.WorkDir, "git", "add", ".")
	integMustRun(t, gitClient.WorkDir, "git", "commit", "-m", "Add auth handler and util")

	dg, err := NewDiffGenerator(gitClient, ReviewConfig{
		RiskPatterns: `internal/auth`,
	}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), baseSHA)
	require.NoError(t, err)

	riskByPath := make(map[string]RiskLevel)
	for _, f := range result.Files {
		riskByPath[f.Path] = f.Risk
	}

	// git always reports paths with forward slashes regardless of the host OS.
	assert.Equal(t, RiskHigh, riskByPath["internal/auth/handler.go"],
		"auth handler should be classified as high-risk")
	assert.Equal(t, RiskNormal, riskByPath["util.go"],
		"util.go should be classified as normal-risk")
}

func TestIntegration_Generate_EmptyDiff_NoChanges(t *testing.T) {
	gitClient, baseSHA := newIntegrationRepo(t)
	// Do NOT add any commits after baseSHA — diff should be empty.

	dg, err := NewDiffGenerator(gitClient, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), baseSHA)
	require.NoError(t, err)

	assert.Empty(t, result.Files)
	assert.Empty(t, result.FullDiff)
	assert.Zero(t, result.Stats.TotalFiles)
	assert.Equal(t, baseSHA, result.BaseBranch)
}

func TestIntegration_Generate_DetachedHEAD(t *testing.T) {
	// Spec requirement: works with detached HEAD (compares against base
	// branch by name rather than a symbolic ref).
	gitClient, _ := newIntegrationRepo(t)
	ctx := context.Background()

	// Create a second commit on main.
	integWriteFile(t, gitClient.WorkDir, "feature.go", "package main\n")
	integMustRun(t, gitClient.WorkDir, "git", "add", ".")
	integMustRun(t, gitClient.WorkDir, "git", "commit", "-m", "Add feature.go")

	headSHA, err := gitClient.HeadCommit(ctx)
	require.NoError(t, err)

	// Detach HEAD at the current commit.
	integMustRun(t, gitClient.WorkDir, "git", "checkout", headSHA)

	dg, err := NewDiffGenerator(gitClient, ReviewConfig{}, nil)
	require.NoError(t, err)

	// Generate should work without error even in detached HEAD state because
	// we pass a commit SHA explicitly (not a symbolic branch ref). Diffing
	// HEAD against itself produces an empty result.
	result, err := dg.Generate(ctx, headSHA)
	require.NoError(t, err)
	assert.Empty(t, result.Files, "diff from HEAD to HEAD should be empty")
}

// ---------------------------------------------------------------------------
// Integration — full pipeline: generate, filter, classify, split
// ---------------------------------------------------------------------------

func TestIntegration_FullPipeline(t *testing.T) {
	gitClient, baseSHA := newIntegrationRepo(t)

	// Prepare a realistic set of changed files.
	require.NoError(t, os.MkdirAll(
		filepath.Join(gitClient.WorkDir, "internal", "auth"), 0o755))
	require.NoError(t, os.MkdirAll(
		filepath.Join(gitClient.WorkDir, "internal", "util"), 0o755))

	integWriteFile(t, gitClient.WorkDir,
		filepath.Join("internal", "auth", "token.go"), // OS-specific for writing
		"package auth\n\nfunc Token() string { return \"\" }\n",
	)
	integWriteFile(t, gitClient.WorkDir,
		filepath.Join("internal", "util", "helper.go"), // OS-specific for writing
		"package util\n\nfunc Helper() {}\n",
	)
	integWriteFile(t, gitClient.WorkDir, "main.go",
		"package main\n\nfunc main() {}\n",
	)
	// A non-Go file that should be filtered out.
	integWriteFile(t, gitClient.WorkDir, "config.json", `{"version":"1"}`+"\n")

	integMustRun(t, gitClient.WorkDir, "git", "add", ".")
	integMustRun(t, gitClient.WorkDir, "git", "commit", "-m", "Full pipeline changes")

	dg, err := NewDiffGenerator(gitClient, ReviewConfig{
		Extensions:   `\.go$`,
		RiskPatterns: `internal/auth`,
	}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), baseSHA)
	require.NoError(t, err)

	// config.json should be filtered out.
	for _, f := range result.Files {
		assert.NotEqual(t, "config.json", f.Path,
			"config.json should be filtered by extension")
	}

	// internal/auth/token.go should be high-risk.
	// git always uses forward slashes in paths regardless of the host OS.
	riskByPath := make(map[string]RiskLevel)
	for _, f := range result.Files {
		riskByPath[f.Path] = f.Risk
	}
	assert.Equal(t, RiskHigh, riskByPath["internal/auth/token.go"],
		"auth/token.go should be high-risk")

	// Stats should reflect filtered files only (3 .go files).
	assert.Equal(t, 3, result.Stats.TotalFiles)
	assert.Equal(t, 1, result.Stats.HighRiskFiles)
	assert.Equal(t, 3, result.Stats.FilesAdded)

	// Split across 2 agents.
	buckets := SplitFiles(result.Files, 2)
	require.Len(t, buckets, 2)

	total := 0
	highRiskInFirst := 0
	for _, f := range buckets[0] {
		total++
		if f.Risk == RiskHigh {
			highRiskInFirst++
		}
	}
	for range buckets[1] {
		total++
	}
	assert.Equal(t, 3, total, "all files must be preserved across buckets")
	assert.Equal(t, 1, highRiskInFirst,
		"high-risk file should be in the first bucket (distributed first)")
}

func TestIntegration_Generate_DiffStatsAccuracy(t *testing.T) {
	gitClient, baseSHA := newIntegrationRepo(t)

	// Modify README (add a line), add a new file, and delete nothing.
	integWriteFile(t, gitClient.WorkDir, "README.md", "# Modified\nLine 2\nLine 3\n")
	integWriteFile(t, gitClient.WorkDir, "added.go", "package main\n\nvar x = 1\n")
	integMustRun(t, gitClient.WorkDir, "git", "add", ".")
	integMustRun(t, gitClient.WorkDir, "git", "commit", "-m", "Modify README and add file")

	dg, err := NewDiffGenerator(gitClient, ReviewConfig{}, nil)
	require.NoError(t, err)

	result, err := dg.Generate(context.Background(), baseSHA)
	require.NoError(t, err)

	assert.Equal(t, 2, result.Stats.TotalFiles)
	assert.Equal(t, 1, result.Stats.FilesAdded)
	assert.Equal(t, 1, result.Stats.FilesModified)
	assert.Equal(t, 0, result.Stats.FilesDeleted)
	assert.Equal(t, 0, result.Stats.FilesRenamed)
	assert.Greater(t, result.Stats.TotalLinesAdded, 0)
}
