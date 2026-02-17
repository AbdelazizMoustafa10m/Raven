package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectRoot returns the absolute path to the project root directory.
func projectRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err, "failed to get working directory")

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (no go.mod found in any parent directory)")
		}
		dir = parent
	}
}

func TestBuild_Compiles(t *testing.T) {
	root := projectRoot(t)
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "raven")

	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/raven/")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(output))

	// Verify the binary was created.
	info, err := os.Stat(binPath)
	require.NoError(t, err, "binary was not created at %s", binPath)
	assert.Greater(t, info.Size(), int64(0), "binary must not be empty")
}

func TestBuild_BinaryRuns(t *testing.T) {
	root := projectRoot(t)
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "raven")

	// Build the binary first.
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/raven/")
	buildCmd.Dir = root
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	buildOutput, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(buildOutput))

	// Run the binary and check it exits with code 0.
	runCmd := exec.Command(binPath)
	output, err := runCmd.CombinedOutput()
	require.NoError(t, err, "binary execution failed with output: %s", string(output))
}

func TestBuild_BinaryOutput(t *testing.T) {
	root := projectRoot(t)
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "raven")

	// Build the binary.
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/raven/")
	buildCmd.Dir = root
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	buildOutput, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(buildOutput))

	// Run the binary and verify output.
	runCmd := exec.Command(binPath)
	output, err := runCmd.CombinedOutput()
	require.NoError(t, err, "binary execution failed")

	outputStr := strings.TrimSpace(string(output))
	assert.Equal(t, "raven - AI workflow orchestration command center", outputStr,
		"binary must print the expected placeholder message")
}

func TestGoRun_Success(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "run", "./cmd/raven/")
	cmd.Dir = root

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go run failed: %s", string(output))

	outputStr := strings.TrimSpace(string(output))
	assert.Equal(t, "raven - AI workflow orchestration command center", outputStr,
		"go run must produce the expected output")
}

func TestGoVet_Passes(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = root

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go vet failed with output: %s", string(output))
}

func TestGoModTidy_NoChanges(t *testing.T) {
	root := projectRoot(t)

	// Read the current go.mod and go.sum content.
	goModBefore, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err, "failed to read go.mod before tidy")

	goSumBefore, err := os.ReadFile(filepath.Join(root, "go.sum"))
	require.NoError(t, err, "failed to read go.sum before tidy")

	// Run go mod tidy.
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = root

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go mod tidy failed: %s", string(output))

	// Read go.mod and go.sum after tidy.
	goModAfter, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err, "failed to read go.mod after tidy")

	goSumAfter, err := os.ReadFile(filepath.Join(root, "go.sum"))
	require.NoError(t, err, "failed to read go.sum after tidy")

	// Verify no changes.
	assert.Equal(t, string(goModBefore), string(goModAfter),
		"go mod tidy should not change go.mod (modules are clean)")
	assert.Equal(t, string(goSumBefore), string(goSumAfter),
		"go mod tidy should not change go.sum (modules are clean)")
}

func TestBuild_CGODisabled(t *testing.T) {
	root := projectRoot(t)
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "raven")

	// Build with CGO_ENABLED=0 per project conventions.
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/raven/")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build with CGO_ENABLED=0 failed: %s", string(output))

	info, err := os.Stat(binPath)
	require.NoError(t, err, "binary not created with CGO_ENABLED=0")
	assert.Greater(t, info.Size(), int64(0), "binary must not be empty")
}
