package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipelineHelp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	out := tp.runExpectSuccess("pipeline", "--help")
	assert.Contains(t, out, "pipeline")
	assert.Contains(t, out, "--phase")
}

func TestPipelineDryRunRequiresPhaseFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	initGitRepo(t, tp.Dir)

	// --dry-run without --phase in non-interactive mode should fail because
	// neither --phase, --from-phase, nor --interactive was provided.
	out, exitCode := tp.runExpectFailure("pipeline", "--dry-run")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestPipelineDryRunWithPhaseAll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	// Write a config that includes a phases.conf path, and create a minimal phases.conf.
	tp.writeConfig(minimalConfig("claude"))
	initGitRepo(t, tp.Dir)

	// Create a minimal phases.conf so the pipeline can resolve phases.
	phasesDir := filepath.Join(tp.Dir, "docs", "tasks")
	require.NoError(t, os.MkdirAll(phasesDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(phasesDir, "phases.conf"),
		[]byte("1|Foundation|T-001|T-001\n"),
		0o644,
	))

	// Also create the task spec referenced by the phase.
	tp.writeTaskSpec("T-001-setup", sampleTaskSpec("T-001", "Setup", nil))

	out := tp.runExpectSuccess("pipeline", "--phase", "all", "--dry-run")
	assert.NotEmpty(t, out)
}

func TestPipelineMutuallyExclusivePhaseFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	initGitRepo(t, tp.Dir)

	// --phase and --from-phase are mutually exclusive.
	out, exitCode := tp.runExpectFailure("pipeline",
		"--phase", "1", "--from-phase", "2", "--dry-run")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestPipelineAllStagesSkippedFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	initGitRepo(t, tp.Dir)

	// Skipping all four stages is rejected as an invalid configuration.
	out, exitCode := tp.runExpectFailure("pipeline",
		"--phase", "1",
		"--skip-implement", "--skip-review", "--skip-fix", "--skip-pr",
		"--dry-run")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestPipelineInvalidReviewConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	initGitRepo(t, tp.Dir)

	out, exitCode := tp.runExpectFailure("pipeline",
		"--phase", "1", "--review-concurrency", "0", "--dry-run")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}
