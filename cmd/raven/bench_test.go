package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// benchRoot returns the absolute path to the project root directory.
// It is equivalent to projectRoot but accepts testing.TB so it works for
// both *testing.T and *testing.B callers.
func benchRoot(tb testing.TB) string {
	tb.Helper()
	dir, err := os.Getwd()
	if err != nil {
		tb.Fatalf("failed to get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			tb.Fatal("could not find project root (no go.mod found in any parent directory)")
		}
		dir = parent
	}
}

// BenchmarkBinaryStartup measures the wall-clock time from process launch to
// exit for "raven version". The binary is built once in the benchmark setup
// and reused for all iterations. This establishes a baseline for the <200ms
// startup time target documented in the performance requirements.
func BenchmarkBinaryStartup(b *testing.B) {
	root := benchRoot(b)
	binDir := b.TempDir()
	binPath := filepath.Join(binDir, "raven")

	// Build the binary once before starting the timer.
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/raven/")
	buildCmd.Dir = root
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		b.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		cmd := exec.Command(binPath, "version")
		if err := cmd.Run(); err != nil {
			b.Fatalf("raven version failed: %v", err)
		}
	}
}

// BenchmarkBinaryHelp measures startup time for "raven --help". This is
// slightly heavier than "version" as it includes help text generation.
func BenchmarkBinaryHelp(b *testing.B) {
	root := benchRoot(b)
	binDir := b.TempDir()
	binPath := filepath.Join(binDir, "raven")

	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/raven/")
	buildCmd.Dir = root
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		b.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		cmd := exec.Command(binPath, "--help")
		// --help exits with code 0 in cobra; ignore the error.
		_ = cmd.Run()
	}
}
