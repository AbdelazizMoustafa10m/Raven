package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	out := tp.runExpectSuccess("version")
	assert.Contains(t, out, "raven")
}

func TestVersionCommandJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	out := tp.runExpectSuccess("version", "--json")
	assert.Contains(t, out, `"version"`)
}

func TestInitCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	// Run raven init in the temp dir (no prior raven.toml).
	cmd := tp.run("init", "go-cli", "--name", "myproject")
	out, err := cmd.CombinedOutput()
	t.Logf("init output: %s (err: %v)", string(out), err)

	// Verify raven.toml was created.
	_, statErr := os.Stat(filepath.Join(tp.Dir, "raven.toml"))
	require.NoError(t, statErr, "raven.toml should be created by init; output:\n%s", string(out))

	// Verify docs/tasks directory exists.
	_, statErr = os.Stat(filepath.Join(tp.Dir, "docs", "tasks"))
	require.NoError(t, statErr, "docs/tasks should be created by init")
}

func TestConfigDebugCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	out := tp.runExpectSuccess("config", "debug")
	assert.Contains(t, out, "Configuration Debug")
	assert.Contains(t, out, "test-project")
}

func TestConfigValidateCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	out := tp.runExpectSuccess("config", "validate")
	// Should produce validation output with no fatal errors.
	assert.Contains(t, out, "Configuration Validation")
}

func TestMissingConfigFallsBackToDefaults(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	// No raven.toml -- config debug should still show defaults.
	out := tp.runExpectSuccess("config", "debug")
	assert.Contains(t, out, "Configuration Debug")
}

func TestNoArgsShowsHelp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	// Cobra's RunE returns cmd.Help() for the root command, which exits 0.
	out := tp.runExpectSuccess()
	assert.Contains(t, out, "raven")
	assert.Contains(t, out, "Usage")
}

func TestConfigHelpSubcommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	out := tp.runExpectSuccess("config", "--help")
	assert.Contains(t, out, "config")
	assert.Contains(t, out, "debug")
	assert.Contains(t, out, "validate")
}
