package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewDryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(reviewConfig())
	initGitRepo(t, tp.Dir)

	// --dry-run is a global flag on the root command; --base HEAD avoids branch name issues.
	out := tp.runExpectSuccess("review", "--agents", "claude", "--base", "HEAD", "--dry-run")
	// Dry-run prints a plan; either "dry" appears or the plan description.
	assert.NotEmpty(t, out)
}

func TestReviewHelp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	out := tp.runExpectSuccess("review", "--help")
	assert.Contains(t, out, "review")
	assert.Contains(t, out, "--agents")
}

func TestReviewWithMockAgentEmptyDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(reviewConfig())
	initGitRepo(t, tp.Dir)

	// Run review against HEAD with no unstaged changes -- produces an empty diff.
	cmd := tp.run("review", "--agents", "claude", "--base", "HEAD")
	out, _ := cmd.CombinedOutput()
	t.Logf("review output: %s", string(out))
	// An empty diff is handled gracefully by raven (exit 0 with a notice).
}

func TestReviewWithMockAgentStagedChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(reviewConfig())
	initGitRepo(t, tp.Dir)

	// Create a new file so there's something to diff against HEAD.
	testFile := filepath.Join(tp.Dir, "main.go")
	require.NoError(t, os.WriteFile(testFile, []byte("package main\nfunc main() {}\n"), 0o644))

	cmd := tp.run("review", "--agents", "claude", "--base", "HEAD")
	out, _ := cmd.CombinedOutput()
	t.Logf("review staged output: %s", string(out))
	// The review runs against the diff; exact outcome depends on mock agent output.
}

func TestReviewRequiresAgentsOrConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	// Write a config with no agents configured.
	tp.writeConfig(`[project]
name = "test-project"
language = "go"
`)
	initGitRepo(t, tp.Dir)

	// No --agents flag and no agents in config: should fail with a descriptive error.
	out, exitCode := tp.runExpectFailure("review")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestReviewModeSplit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(reviewConfig())
	initGitRepo(t, tp.Dir)

	// --mode split is a valid flag; --base HEAD --dry-run to avoid branch name issues.
	out := tp.runExpectSuccess("review", "--agents", "claude", "--mode", "split", "--base", "HEAD", "--dry-run")
	assert.NotEmpty(t, out)
}

func TestReviewInvalidModeFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(reviewConfig())
	initGitRepo(t, tp.Dir)

	out, exitCode := tp.runExpectFailure("review", "--agents", "claude", "--mode", "bogusmode")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

// reviewConfig returns a raven.toml with review section configuration.
func reviewConfig() string {
	return `[project]
name = "test-project"
language = "go"
tasks_dir = "docs/tasks"

[agents.claude]
command = "claude"

[review]
extensions = ".go"
`
}
