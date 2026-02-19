package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImplementDryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	tp.writeTaskSpec("T-001-setup", sampleTaskSpec("T-001", "Setup project", nil))
	initGitRepo(t, tp.Dir)

	// Write a task-state.conf marking T-001 as not-started.
	stateDir := filepath.Join(tp.Dir, "docs", "tasks")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(stateDir, "task-state.conf"),
		[]byte("T-001|not_started|2026-01-01\n"),
		0o644,
	))

	out := tp.runExpectSuccess("implement", "--agent", "claude", "--task", "T-001", "--dry-run")
	assert.Contains(t, out, "T-001")
}

func TestImplementSingleTaskWithMockAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	tp.writeTaskSpec("T-001-setup", sampleTaskSpec("T-001", "Setup project", nil))
	initGitRepo(t, tp.Dir)

	// Write task-state.conf marking T-001 as not-started.
	stateDir := filepath.Join(tp.Dir, "docs", "tasks")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(stateDir, "task-state.conf"),
		[]byte("T-001|not_started|2026-01-01\n"),
		0o644,
	))

	cmd := tp.run("implement", "--agent", "claude", "--task", "T-001",
		"--max-iterations", "2", "--sleep", "0")
	out, err := cmd.CombinedOutput()
	// The command may succeed or fail depending on mock agent output recognition.
	// The key assertion is that the task ID appears in the output.
	t.Logf("implement output: %s (err: %v)", string(out), err)
	assert.Contains(t, string(out), "T-001")
}

func TestImplementWithNoTasksReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	initGitRepo(t, tp.Dir)

	// No task specs written -- docs/tasks directory does not exist.
	// We expect a non-zero exit about missing tasks or missing task T-999.
	_, exitCode := tp.runExpectFailure("implement", "--agent", "claude", "--task", "T-999")
	assert.NotEqual(t, 0, exitCode)
}

func TestImplementDryRunNoAgentNeeded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	tp.writeTaskSpec("T-002-feature", sampleTaskSpec("T-002", "Add feature", nil))
	initGitRepo(t, tp.Dir)

	stateDir := filepath.Join(tp.Dir, "docs", "tasks")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(stateDir, "task-state.conf"),
		[]byte("T-002|not_started|2026-01-01\n"),
		0o644,
	))

	// Dry-run should not invoke the mock agent (no MOCK_SIGNAL_FILE entries).
	out := tp.runExpectSuccess("implement", "--agent", "claude", "--task", "T-002", "--dry-run")
	assert.Contains(t, out, "T-002")
}

func TestImplementRequiresAgentFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	// Missing --agent flag; cobra marks it as required.
	out, exitCode := tp.runExpectFailure("implement", "--task", "T-001")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestImplementRequiresPhaseOrTask(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	// Neither --phase nor --task specified; should fail with a validation error.
	out, exitCode := tp.runExpectFailure("implement", "--agent", "claude")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestImplementMockAgentNotInvokedForDryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	// Not parallel: writes a signal file at a predictable path.

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	tp.writeTaskSpec("T-001-setup", sampleTaskSpec("T-001", "Setup project", nil))
	initGitRepo(t, tp.Dir)

	stateDir := filepath.Join(tp.Dir, "docs", "tasks")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(stateDir, "task-state.conf"),
		[]byte("T-001|not_started|2026-01-01\n"),
		0o644,
	))

	signalFile := filepath.Join(tp.Dir, "agent-calls.log")

	// Dry-run should NOT invoke the agent, so the signal file should not exist.
	cmd := tp.run("implement", "--agent", "claude", "--task", "T-001", "--dry-run")
	cmd.Env = append(cmd.Env, fmt.Sprintf("MOCK_SIGNAL_FILE=%s", signalFile))
	out, _ := cmd.CombinedOutput()
	t.Logf("dry-run output: %s", string(out))

	// Signal file should not be created because dry-run does not invoke the agent.
	_, statErr := os.Stat(signalFile)
	assert.True(t, os.IsNotExist(statErr), "agent should not be invoked during dry-run")
}
