package e2e_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// testProject creates an isolated project directory with raven.toml and mock agents.
type testProject struct {
	Dir        string
	BinaryPath string
	t          *testing.T
}

// newTestProject builds the raven binary, copies mock agents into a fresh temp
// directory, and returns a testProject ready for use. Must be called from a
// test function; uses t.Helper() to mark itself accordingly.
func newTestProject(t *testing.T) *testProject {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("E2E tests with bash mock agents are not supported on Windows")
	}

	dir := t.TempDir()

	// Build raven binary into temp dir.
	binary := filepath.Join(dir, "raven")
	build := exec.Command("go", "build", "-o", binary, "./cmd/raven")
	build.Dir = projectRoot()
	out, err := build.CombinedOutput()
	require.NoError(t, err, "building raven: %s", string(out))

	// Copy mock agents and make them executable.
	mockDir := filepath.Join(dir, "mock-agents")
	copyMockAgents(t, mockDir)

	// Create the default prompts directory expected by the config defaults.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "prompts"), 0o755))

	return &testProject{Dir: dir, BinaryPath: binary, t: t}
}

// projectRoot returns the absolute path to the root of the raven repository.
// It uses runtime.Caller(0) to find this source file's location and navigates
// two directories up (tests/e2e/ -> tests/ -> repo root).
func projectRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile is <repo>/tests/e2e/helpers_test.go; root is two dirs up.
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// copyMockAgents copies all scripts from testdata/mock-agents/ to destDir,
// setting executable permissions on each script.
func copyMockAgents(t *testing.T, destDir string) {
	t.Helper()
	srcDir := filepath.Join(projectRoot(), "testdata", "mock-agents")
	require.NoError(t, os.MkdirAll(destDir, 0o755))

	entries, err := os.ReadDir(srcDir)
	require.NoError(t, err, "reading mock-agents dir")

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src := filepath.Join(srcDir, entry.Name())
		dst := filepath.Join(destDir, entry.Name())
		data, readErr := os.ReadFile(src)
		require.NoError(t, readErr)
		require.NoError(t, os.WriteFile(dst, data, 0o600))
		require.NoError(t, os.Chmod(dst, 0o755))
	}
}

// writeConfig writes content to raven.toml in tp.Dir.
func (tp *testProject) writeConfig(content string) {
	tp.t.Helper()
	err := os.WriteFile(filepath.Join(tp.Dir, "raven.toml"), []byte(content), 0o644)
	require.NoError(tp.t, err)
}

// writeTaskSpec writes a task markdown file to docs/tasks/<id>.md.
func (tp *testProject) writeTaskSpec(id, content string) {
	tp.t.Helper()
	tasksDir := filepath.Join(tp.Dir, "docs", "tasks")
	require.NoError(tp.t, os.MkdirAll(tasksDir, 0o755))
	err := os.WriteFile(filepath.Join(tasksDir, id+".md"), []byte(content), 0o644)
	require.NoError(tp.t, err)
}

// run creates an exec.Cmd for raven with mock agents prepended to PATH.
func (tp *testProject) run(args ...string) *exec.Cmd {
	cmd := exec.Command(tp.BinaryPath, args...)
	cmd.Dir = tp.Dir
	mockPath := filepath.Join(tp.Dir, "mock-agents")
	cmd.Env = append(os.Environ(),
		"PATH="+mockPath+string(os.PathListSeparator)+os.Getenv("PATH"),
		"NO_COLOR=1",            // disable ANSI color in output
		"RAVEN_LOG_FORMAT=json", // structured logs for easier parsing
	)
	return cmd
}

// runExpectSuccess runs raven and asserts exit code 0.
// Returns combined stdout+stderr output.
func (tp *testProject) runExpectSuccess(args ...string) string {
	tp.t.Helper()
	cmd := tp.run(args...)
	out, err := cmd.CombinedOutput()
	require.NoError(tp.t, err, "raven %v failed:\n%s", args, string(out))
	return string(out)
}

// runExpectFailure runs raven and asserts a non-zero exit code.
// Returns combined output and the exit code.
func (tp *testProject) runExpectFailure(args ...string) (string, int) {
	tp.t.Helper()
	cmd := tp.run(args...)
	out, err := cmd.CombinedOutput()
	require.Error(tp.t, err, "raven %v expected to fail but succeeded:\n%s", args, string(out))
	var exitErr *exec.ExitError
	require.True(tp.t, errors.As(err, &exitErr), "expected *exec.ExitError, got %T: %v", err, err)
	return string(out), exitErr.ExitCode()
}

// initGitRepo initialises a git repository in dir with an initial commit.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	setupCmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range setupCmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%v failed: %s", args, string(out))
	}

	// Create a .gitkeep and make an initial commit so the repo is non-empty.
	keepFile := filepath.Join(dir, ".gitkeep")
	require.NoError(t, os.WriteFile(keepFile, []byte(""), 0o644))
	for _, args := range [][]string{
		{"git", "add", ".gitkeep"},
		{"git", "commit", "-m", "init"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%v failed: %s", args, string(out))
	}
}

// minimalConfig returns a minimal raven.toml content that points to a single
// mock agent by name (e.g., "claude").
func minimalConfig(agentName string) string {
	return fmt.Sprintf(`[project]
name = "test-project"
language = "go"
tasks_dir = "docs/tasks"
task_state_file = "docs/tasks/task-state.conf"
phases_conf = "docs/tasks/phases.conf"
progress_file = "docs/tasks/PROGRESS.md"

[agents.%s]
command = "%s"
`, agentName, agentName)
}

// sampleTaskSpec returns minimal task spec markdown content for use in tests.
// deps is an optional list of dependency task IDs (e.g., []string{"T-000"}).
func sampleTaskSpec(id, name string, deps []string) string {
	depLine := ""
	if len(deps) > 0 {
		depLine = "| Dependencies | " + strings.Join(deps, ", ") + " |\n"
	}
	return fmt.Sprintf(`# %s: %s

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
%s
## Goal
Test task for E2E testing.

## Acceptance Criteria
- [ ] %s is implemented
`, id, name, depLine, name)
}
