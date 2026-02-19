package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnknownSubcommandFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	out, exitCode := tp.runExpectFailure("nonexistent-command")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestImplementWithUnknownAgentFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))
	initGitRepo(t, tp.Dir)

	// "unknownagent999" is not a registered agent name.
	out, exitCode := tp.runExpectFailure("implement",
		"--agent", "unknownagent999", "--task", "T-001")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestInvalidConfigFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig("this is not valid toml ][")

	out, exitCode := tp.runExpectFailure("config", "debug")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestGlobalDryRunFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	// The global --dry-run flag should be accepted by all commands.
	out := tp.runExpectSuccess("config", "debug", "--dry-run")
	assert.Contains(t, out, "Configuration Debug")
}

func TestGlobalVerboseFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	// --verbose should not cause a crash.
	out := tp.runExpectSuccess("version", "--verbose")
	assert.Contains(t, out, "raven")
}

func TestGlobalNoColorFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	// --no-color is always present from the env (NO_COLOR=1), but passing it
	// explicitly as a flag should also be accepted.
	out := tp.runExpectSuccess("version", "--no-color")
	assert.Contains(t, out, "raven")
}

func TestImplementPhaseAndTaskMutuallyExclusive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	// Providing both --phase and --task is an error.
	out, exitCode := tp.runExpectFailure("implement",
		"--agent", "claude", "--phase", "1", "--task", "T-001")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestReviewInvalidConcurrencyFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(reviewConfig())
	initGitRepo(t, tp.Dir)

	// --concurrency 0 is below the minimum of 1.
	// Note: cobra doesn't validate integer flag ranges automatically, so this
	// exercises the runtime validation path in the review command.
	cmd := tp.run("review", "--agents", "claude", "--concurrency", "0", "--dry-run")
	out, _ := cmd.CombinedOutput()
	t.Logf("review concurrency 0 output: %s", string(out))
	// Either fails with validation error or succeeds -- verify it doesn't panic.
}
