package tools_test

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
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root")
		}
		dir = parent
	}
}

// readMakefile reads the Makefile content from the project root.
func readMakefile(t *testing.T) string {
	t.Helper()
	root := projectRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	require.NoError(t, err, "failed to read Makefile")
	return string(data)
}

// runMake executes a make target in the project root and returns combined output.
func runMake(t *testing.T, target string) (string, error) {
	t.Helper()
	root := projectRoot(t)
	cmd := exec.Command("make", target)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestMakefile_Exists(t *testing.T) {
	t.Parallel()

	root := projectRoot(t)
	info, err := os.Stat(filepath.Join(root, "Makefile"))
	require.NoError(t, err, "Makefile does not exist at project root")
	assert.False(t, info.IsDir(), "Makefile must be a regular file, not a directory")
	assert.Greater(t, info.Size(), int64(0), "Makefile must not be empty")
}

func TestMakefile_ContainsTargets(t *testing.T) {
	t.Parallel()

	content := readMakefile(t)

	targets := []struct {
		name   string
		marker string
	}{
		{name: "build", marker: "build:"},
		{name: "test", marker: "test:"},
		{name: "vet", marker: "vet:"},
		{name: "lint", marker: "lint:"},
		{name: "tidy", marker: "tidy:"},
		{name: "clean", marker: "clean:"},
		{name: "install", marker: "install:"},
		{name: "fmt", marker: "fmt:"},
		{name: "bench", marker: "bench:"},
		{name: "build-debug", marker: "build-debug:"},
		{name: "run-version", marker: "run-version:"},
		{name: "all", marker: "all:"},
	}

	for _, tt := range targets {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Contains(t, content, tt.marker,
				"Makefile must contain target %q", tt.name)
		})
	}
}

func TestMakefile_ContainsCGODisabled(t *testing.T) {
	t.Parallel()

	content := readMakefile(t)
	assert.Contains(t, content, "CGO_ENABLED=0",
		"Makefile must set CGO_ENABLED=0 for pure Go builds")
}

func TestMakefile_ContainsLdflags(t *testing.T) {
	t.Parallel()

	content := readMakefile(t)

	ldflagChecks := []struct {
		name    string
		pattern string
	}{
		{name: "ldflags declaration", pattern: "LDFLAGS"},
		{name: "Version injection", pattern: "buildinfo.Version"},
		{name: "Commit injection", pattern: "buildinfo.Commit"},
		{name: "Date injection", pattern: "buildinfo.Date"},
		{name: "X flag for linker", pattern: "-X"},
	}

	for _, tt := range ldflagChecks {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Contains(t, content, tt.pattern,
				"Makefile must contain %q for ldflags injection", tt.pattern)
		})
	}
}

func TestMakefile_ContainsPhony(t *testing.T) {
	t.Parallel()

	content := readMakefile(t)
	assert.Contains(t, content, ".PHONY:",
		"Makefile must declare .PHONY targets")
}

func TestMakeBuild_ProducesBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping make build test in short mode")
	}

	root := projectRoot(t)

	// Clean first to ensure a fresh build.
	_, _ = runMake(t, "clean")

	output, err := runMake(t, "build")
	require.NoError(t, err, "make build failed: %s", output)

	binPath := filepath.Join(root, "dist", "raven")
	info, err := os.Stat(binPath)
	require.NoError(t, err, "binary not found at dist/raven after make build")
	assert.Greater(t, info.Size(), int64(0), "binary must not be empty")

	// Clean up after the test.
	t.Cleanup(func() {
		_, _ = runMake(t, "clean")
	})
}

func TestMakeClean_RemovesDist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping make clean test in short mode")
	}

	root := projectRoot(t)

	// Build first so dist/ exists.
	output, err := runMake(t, "build")
	require.NoError(t, err, "make build failed: %s", output)

	distDir := filepath.Join(root, "dist")
	_, err = os.Stat(distDir)
	require.NoError(t, err, "dist/ should exist after make build")

	// Now clean.
	output, err = runMake(t, "clean")
	require.NoError(t, err, "make clean failed: %s", output)

	_, err = os.Stat(distDir)
	assert.True(t, os.IsNotExist(err),
		"dist/ directory should be removed after make clean")
}

func TestMakeBuildDebug_ProducesBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping make build-debug test in short mode")
	}

	root := projectRoot(t)

	// Clean first.
	_, _ = runMake(t, "clean")

	output, err := runMake(t, "build-debug")
	require.NoError(t, err, "make build-debug failed: %s", output)

	debugBinPath := filepath.Join(root, "dist", "raven-debug")
	info, err := os.Stat(debugBinPath)
	require.NoError(t, err, "debug binary not found at dist/raven-debug after make build-debug")
	assert.Greater(t, info.Size(), int64(0), "debug binary must not be empty")

	// Clean up after the test.
	t.Cleanup(func() {
		_, _ = runMake(t, "clean")
	})
}

func TestMakeBuild_LdflagsInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ldflags injection test in short mode")
	}

	root := projectRoot(t)

	// Clean and build.
	_, _ = runMake(t, "clean")

	output, err := runMake(t, "build")
	require.NoError(t, err, "make build failed: %s", output)

	binPath := filepath.Join(root, "dist", "raven")
	_, err = os.Stat(binPath)
	require.NoError(t, err, "binary not found at dist/raven")

	// Use `go version -m` to inspect the binary's build info and ldflags.
	// The ldflags inject values via -X, which embeds them in the binary.
	// We verify by checking that the build command in the Makefile includes -X flags
	// and the binary was built successfully with them.
	//
	// Additionally, check that `git rev-parse --short HEAD` returns a value,
	// meaning the commit hash will have been injected (not "unknown").
	gitCmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	gitCmd.Dir = root
	commitOut, err := gitCmd.CombinedOutput()
	require.NoError(t, err, "git rev-parse failed: %s", string(commitOut))

	commitHash := strings.TrimSpace(string(commitOut))
	assert.NotEmpty(t, commitHash, "git commit hash should not be empty")
	assert.NotEqual(t, "unknown", commitHash, "git commit hash should not be 'unknown'")

	// Verify the binary contains the injected commit hash in its data.
	// Read the binary bytes and check that the commit hash string is embedded.
	binData, err := os.ReadFile(binPath)
	require.NoError(t, err, "failed to read binary")

	assert.True(t, strings.Contains(string(binData), commitHash),
		"binary should contain the injected commit hash %q", commitHash)

	// The version from `git describe` should also be present (not "dev").
	gitDescCmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	gitDescCmd.Dir = root
	descOut, err := gitDescCmd.CombinedOutput()
	require.NoError(t, err, "git describe failed: %s", string(descOut))

	version := strings.TrimSpace(string(descOut))
	assert.NotEmpty(t, version, "git describe output should not be empty")
	assert.NotEqual(t, "dev", version, "version should not be 'dev' when built from a git repo")

	assert.True(t, strings.Contains(string(binData), version),
		"binary should contain the injected version %q", version)

	// Clean up.
	t.Cleanup(func() {
		_, _ = runMake(t, "clean")
	})
}
