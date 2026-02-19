package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPRDHelp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	out := tp.runExpectSuccess("prd", "--help")
	assert.Contains(t, out, "prd")
	assert.Contains(t, out, "--file")
}

func TestPRDRequiresFileFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	// --file is a required flag.
	out, exitCode := tp.runExpectFailure("prd")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestPRDFileMustExist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	// Point --file to a non-existent path.
	out, exitCode := tp.runExpectFailure("prd", "--file", "/nonexistent/PRD.md")
	assert.NotEqual(t, 0, exitCode)
	_ = out
}

func TestPRDDryRunWithMockAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	// Create a minimal PRD file.
	docsDir := filepath.Join(tp.Dir, "docs", "prd")
	require.NoError(t, os.MkdirAll(docsDir, 0o755))
	prdContent := `# Test PRD

## Overview
A simple test project.

## Features
- Feature 1: Basic setup
`
	prdPath := filepath.Join(docsDir, "PRD-test.md")
	require.NoError(t, os.WriteFile(prdPath, []byte(prdContent), 0o644))

	outputDir := filepath.Join(tp.Dir, "docs", "tasks")
	cmd := tp.run("prd",
		"--file", prdPath,
		"--agent", "claude",
		"--output-dir", outputDir,
		"--dry-run")
	out, err := cmd.CombinedOutput()
	t.Logf("prd decompose output: %s (err: %v)", string(out), err)

	// Dry-run should succeed (agent prerequisite check is skipped in dry-run mode).
	// The key assertion: output contains "Dry Run" indicating dry-run was triggered.
	// Note: prd dry-run writes to stderr; CombinedOutput captures both.
	assert.Contains(t, string(out), "Dry Run")
}

func TestPRDDryRunSinglePass(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	t.Parallel()

	tp := newTestProject(t)
	tp.writeConfig(minimalConfig("claude"))

	docsDir := filepath.Join(tp.Dir, "docs", "prd")
	require.NoError(t, os.MkdirAll(docsDir, 0o755))
	prdPath := filepath.Join(docsDir, "PRD.md")
	require.NoError(t, os.WriteFile(prdPath, []byte("# PRD\n## Overview\nTest.\n"), 0o644))

	outputDir := filepath.Join(tp.Dir, "docs", "tasks")
	cmd := tp.run("prd",
		"--file", prdPath,
		"--agent", "claude",
		"--output-dir", outputDir,
		"--single-pass",
		"--dry-run")
	out, _ := cmd.CombinedOutput()
	t.Logf("prd single-pass dry-run output: %s", string(out))

	assert.Contains(t, string(out), "Dry Run")
}
