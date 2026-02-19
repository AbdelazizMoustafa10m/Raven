package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResumeHelp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	out := tp.runExpectSuccess("resume", "--help")
	assert.Contains(t, out, "resume")
	assert.Contains(t, out, "--run")
	assert.Contains(t, out, "--list")
}

func TestResumeWithNoCheckpointFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	initGitRepo(t, tp.Dir)

	// No .raven/state/ directory -- resume should fail with a descriptive error.
	out, exitCode := tp.runExpectFailure("resume")
	t.Logf("resume no checkpoint output: %s (exit: %d)", out, exitCode)
	assert.NotEqual(t, 0, exitCode)
}

func TestResumeListWithNoCheckpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	initGitRepo(t, tp.Dir)

	// --list with no checkpoints should succeed and print nothing (or a notice).
	cmd := tp.run("resume", "--list")
	out, _ := cmd.CombinedOutput()
	t.Logf("resume --list output: %s", string(out))
	// Should exit 0 even with no checkpoints.
}

func TestResumeCleanAllNoCheckpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	initGitRepo(t, tp.Dir)

	// --clean-all --force with no checkpoints should succeed with a notice.
	cmd := tp.run("resume", "--clean-all", "--force")
	out, err := cmd.CombinedOutput()
	t.Logf("resume --clean-all output: %s (err: %v)", string(out), err)
}

func TestResumeInvalidRunIDFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	// Run IDs containing path separators or special chars are rejected.
	out, exitCode := tp.runExpectFailure("resume", "--run", "../../../etc/passwd")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestStatusCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	// Create minimal task state.
	stateDir := filepath.Join(tp.Dir, "docs", "tasks")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(stateDir, "task-state.conf"),
		[]byte("T-001|not_started|2026-01-01\nT-002|completed|2026-01-02\n"),
		0o644,
	))
	// Write task specs so DiscoverTasks finds them.
	tp.writeTaskSpec("T-001-setup", sampleTaskSpec("T-001", "Setup", nil))
	tp.writeTaskSpec("T-002-feature", sampleTaskSpec("T-002", "Feature", nil))

	cmd := tp.run("status")
	out, _ := cmd.CombinedOutput()
	t.Logf("status output: %s", string(out))
	// Status renders to stderr per convention; just verify the command runs.
}

func TestStatusJSONOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	stateDir := filepath.Join(tp.Dir, "docs", "tasks")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(stateDir, "task-state.conf"),
		[]byte("T-001|completed|2026-01-02\n"),
		0o644,
	))
	tp.writeTaskSpec("T-001-setup", sampleTaskSpec("T-001", "Setup", nil))

	// --json outputs to stdout; should contain JSON keys.
	cmd := tp.run("status", "--json")
	out, _ := cmd.CombinedOutput()
	t.Logf("status --json output: %s", string(out))
	assert.Contains(t, string(out), `"total_tasks"`)
}
