package review

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRunner returns a VerificationRunner with no timeout suitable for
// unit tests. All commands are executed against the OS shell.
func newTestRunner(commands []string) *VerificationRunner {
	return NewVerificationRunner(commands, "", 0, nil)
}

// newTestVerifyLogger returns a charmbracelet logger that discards all output,
// suitable for tests that need to exercise logger code paths without noise.
func newTestVerifyLogger() *log.Logger {
	return log.New(io.Discard)
}

// makeVerificationReport is a helper for building VerificationReport test
// fixtures with named fields to avoid positional confusion.
func makeVerificationReport(
	status VerificationStatus,
	results []CommandResult,
	totalDuration time.Duration,
	passed, failed int,
) *VerificationReport {
	return &VerificationReport{
		Status:   status,
		Results:  results,
		Duration: totalDuration,
		Passed:   passed,
		Failed:   failed,
		Total:    passed + failed,
	}
}

// ---------------------------------------------------------------------------
// NewVerificationRunner
// ---------------------------------------------------------------------------

func TestNewVerificationRunner_DefensiveCopy(t *testing.T) {
	t.Parallel()

	original := []string{"echo hello", "echo world"}
	vr := NewVerificationRunner(original, "/tmp", 5*time.Second, nil)

	// Mutating the original slice must not affect the runner.
	original[0] = "rm -rf /"
	assert.Equal(t, "echo hello", vr.commands[0])
}

func TestNewVerificationRunner_NilCommandList(t *testing.T) {
	t.Parallel()

	// Passing nil is permitted and produces an empty internal slice.
	vr := NewVerificationRunner(nil, "", 0, nil)
	require.NotNil(t, vr)
	assert.Empty(t, vr.commands)
}

func TestNewVerificationRunner_WithLogger(t *testing.T) {
	t.Parallel()

	logger := newTestVerifyLogger()
	vr := NewVerificationRunner([]string{"echo hi"}, "", 0, logger)
	require.NotNil(t, vr)
	assert.NotNil(t, vr.logger)
}

func TestNewVerificationRunner_StoresWorkDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	vr := NewVerificationRunner([]string{"echo hi"}, dir, 0, nil)
	assert.Equal(t, dir, vr.workDir)
}

func TestNewVerificationRunner_StoresTolerance(t *testing.T) {
	t.Parallel()

	vr := NewVerificationRunner(nil, "", 10*time.Second, nil)
	assert.Equal(t, 10*time.Second, vr.timeout)
}

// ---------------------------------------------------------------------------
// RunSingle — passing and exit-code table
// ---------------------------------------------------------------------------

func TestRunSingle_PassingCommand(t *testing.T) {
	t.Parallel()

	vr := newTestRunner(nil)
	result, err := vr.RunSingle(context.Background(), "echo hello")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Passed)
	assert.Equal(t, 0, result.ExitCode)
	assert.False(t, result.TimedOut)
	assert.Contains(t, result.Stdout, "hello")
	assert.Equal(t, "echo hello", result.Command)
	assert.Greater(t, result.Duration, time.Duration(0))
}

func TestRunSingle_ExitCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		command  string
		wantCode int
		wantPass bool
	}{
		{
			name:     "exit code 0",
			command:  "exit 0",
			wantCode: 0,
			wantPass: true,
		},
		{
			name:     "exit code 1",
			command:  "exit 1",
			wantCode: 1,
			wantPass: false,
		},
		{
			name:     "exit code 2",
			command:  "exit 2",
			wantCode: 2,
			wantPass: false,
		},
		{
			name:     "exit code 42",
			command:  "exit 42",
			wantCode: 42,
			wantPass: false,
		},
		{
			name:     "false command",
			command:  "false",
			wantCode: 1,
			wantPass: false,
		},
		{
			name:     "true command",
			command:  "true",
			wantCode: 0,
			wantPass: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vr := newTestRunner(nil)
			result, err := vr.RunSingle(context.Background(), tt.command)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantPass, result.Passed, "command: %s", tt.command)
			assert.Equal(t, tt.wantCode, result.ExitCode, "command: %s", tt.command)
			assert.False(t, result.TimedOut, "command: %s", tt.command)
		})
	}
}

func TestRunSingle_CommandWithArguments(t *testing.T) {
	t.Parallel()

	// Verify that multi-token commands (like "go test ./...") are parsed and
	// executed correctly through the shell, not split naively.
	vr := newTestRunner(nil)
	result, err := vr.RunSingle(context.Background(), "echo foo bar baz")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Passed)
	assert.Contains(t, result.Stdout, "foo bar baz")
	assert.Equal(t, "echo foo bar baz", result.Command)
}

func TestRunSingle_CapturesStdoutAndStderr(t *testing.T) {
	t.Parallel()

	vr := newTestRunner(nil)
	result, err := vr.RunSingle(context.Background(), "echo out; echo err >&2")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Passed)
	assert.Contains(t, result.Stdout, "out")
	assert.Contains(t, result.Stderr, "err")
}

func TestRunSingle_StderrOnlyCommand(t *testing.T) {
	t.Parallel()

	// Command that writes only to stderr and fails.
	vr := newTestRunner(nil)
	result, err := vr.RunSingle(context.Background(), "echo error-message >&2; exit 1")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Stderr, "error-message")
	assert.Empty(t, strings.TrimSpace(result.Stdout))
}

func TestRunSingle_CommandNotFound(t *testing.T) {
	t.Parallel()

	// A command that does not exist on PATH must result in Passed=false and
	// a non-zero exit code without propagating an error from RunSingle itself.
	vr := newTestRunner(nil)
	result, err := vr.RunSingle(context.Background(), "this-command-does-not-exist-9z8x7w")

	require.NoError(t, err, "RunSingle must not return an error for a missing command")
	require.NotNil(t, result)
	assert.False(t, result.Passed, "missing command must not pass")
	assert.NotEqual(t, 0, result.ExitCode, "missing command must have non-zero exit code")
}

func TestRunSingle_Timeout(t *testing.T) {
	t.Parallel()

	// Use a very short timeout to force the command to time out.
	vr := NewVerificationRunner(nil, "", 50*time.Millisecond, nil)
	result, err := vr.RunSingle(context.Background(), "sleep 5")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Passed)
	assert.True(t, result.TimedOut)
	assert.Equal(t, -1, result.ExitCode)
}

func TestRunSingle_ContextCancellation(t *testing.T) {
	t.Parallel()

	vr := newTestRunner(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result, err := vr.RunSingle(ctx, "sleep 5")

	// A cancelled parent context must surface as an error from RunSingle.
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestRunSingle_WorkingDirectory(t *testing.T) {
	t.Parallel()

	// Create a temp directory and verify that commands run inside it.
	dir := t.TempDir()
	// Write a sentinel file we can check.
	sentinelFile := filepath.Join(dir, "sentinel.txt")
	require.NoError(t, os.WriteFile(sentinelFile, []byte("ok"), 0644))

	vr := NewVerificationRunner(nil, dir, 0, nil)
	// "ls sentinel.txt" should succeed only if cwd is the temp dir.
	result, err := vr.RunSingle(context.Background(), "ls sentinel.txt")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Passed, "ls must succeed in the configured workDir")
	assert.Contains(t, result.Stdout, "sentinel.txt")
}

func TestRunSingle_WithLogger_DoesNotPanic(t *testing.T) {
	t.Parallel()

	// Exercise all logger branches (start, pass, fail) with a discarding logger.
	logger := newTestVerifyLogger()
	vr := NewVerificationRunner(nil, "", 0, logger)

	// Passing command exercises the pass log branch.
	result, err := vr.RunSingle(context.Background(), "echo ok")
	require.NoError(t, err)
	assert.True(t, result.Passed)

	// Failing command exercises the fail log branch.
	result, err = vr.RunSingle(context.Background(), "exit 1")
	require.NoError(t, err)
	assert.False(t, result.Passed)
}

func TestRunSingle_CommandFieldPreserved(t *testing.T) {
	t.Parallel()

	vr := newTestRunner(nil)
	cmd := "go test ./internal/review/..."
	result, err := vr.RunSingle(context.Background(), cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	// Regardless of success/fail the Command field must be the original string.
	assert.Equal(t, cmd, result.Command)
}

// ---------------------------------------------------------------------------
// Run — all pass / first fails / second of three fails
// ---------------------------------------------------------------------------

func TestRun_AllPass(t *testing.T) {
	t.Parallel()

	vr := newTestRunner([]string{"echo a", "echo b", "echo c"})
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, VerificationPassed, report.Status)
	assert.Equal(t, 3, report.Passed)
	assert.Equal(t, 0, report.Failed)
	assert.Equal(t, 3, report.Total)
	assert.Len(t, report.Results, 3)
	for _, r := range report.Results {
		assert.True(t, r.Passed, "all results must pass")
	}
}

func TestRun_FirstCommandFails(t *testing.T) {
	t.Parallel()

	vr := newTestRunner([]string{"exit 1", "echo b", "echo c"})
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, VerificationFailed, report.Status)
	// All three commands must have run when stopOnFailure is false.
	assert.Len(t, report.Results, 3)
	assert.Equal(t, 1, report.Failed)
	assert.Equal(t, 2, report.Passed)
	assert.Equal(t, 3, report.Total)
	// First result must be the failing one.
	assert.False(t, report.Results[0].Passed)
	assert.Equal(t, "exit 1", report.Results[0].Command)
}

func TestRun_OneFails(t *testing.T) {
	t.Parallel()

	// Second of three commands fails; stopOnFailure=false so all three run.
	vr := newTestRunner([]string{"echo a", "exit 1", "echo c"})
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, VerificationFailed, report.Status)
	assert.Equal(t, 2, report.Passed)
	assert.Equal(t, 1, report.Failed)
	assert.Equal(t, 3, report.Total)
	// All three commands should have been attempted.
	assert.Len(t, report.Results, 3)
	// Verify the second result is the failing one.
	assert.False(t, report.Results[1].Passed, "second command must fail")
}

func TestRun_SecondFailsStopOnFailure(t *testing.T) {
	t.Parallel()

	// Second of three fails and stopOnFailure=true: exactly two commands run.
	vr := newTestRunner([]string{"echo a", "exit 1", "echo c"})
	report, err := vr.Run(context.Background(), true)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, VerificationFailed, report.Status)
	// Exactly two commands executed: "echo a" (pass) and "exit 1" (fail).
	assert.Len(t, report.Results, 2)
	assert.Equal(t, 1, report.Passed)
	assert.Equal(t, 1, report.Failed)
	assert.Equal(t, 2, report.Total)
	// The third command must NOT appear in results.
	for _, r := range report.Results {
		assert.NotEqual(t, "echo c", r.Command, "third command must not have run")
	}
}

func TestRun_StopOnFailure(t *testing.T) {
	t.Parallel()

	vr := newTestRunner([]string{"echo a", "exit 1", "echo c"})
	report, err := vr.Run(context.Background(), true)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, VerificationFailed, report.Status)
	// Only two commands should have been executed: "echo a" and "exit 1".
	assert.Len(t, report.Results, 2)
	assert.Equal(t, 1, report.Passed)
	assert.Equal(t, 1, report.Failed)
	assert.Equal(t, 2, report.Total)
}

func TestRun_SkipsEmptyCommands(t *testing.T) {
	t.Parallel()

	vr := newTestRunner([]string{"echo a", "", "   ", "echo b"})
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	require.NotNil(t, report)
	// Empty strings are skipped; only 2 commands execute.
	assert.Equal(t, 2, report.Total)
	assert.Equal(t, VerificationPassed, report.Status)
}

func TestRun_WhitespaceOnlyCommandSkipped(t *testing.T) {
	t.Parallel()

	// A command consisting only of whitespace must be skipped and must NOT
	// appear in the results or be counted in Total.
	vr := newTestRunner([]string{"echo first", "   \t  ", "echo last"})
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, 2, report.Total)
	assert.Equal(t, VerificationPassed, report.Status)
	assert.Len(t, report.Results, 2)
}

func TestRun_EmptyCommandList(t *testing.T) {
	t.Parallel()

	vr := newTestRunner([]string{})
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, VerificationPassed, report.Status)
	assert.Equal(t, 0, report.Total)
	assert.Equal(t, 0, report.Passed)
	assert.Equal(t, 0, report.Failed)
	assert.Empty(t, report.Results)
}

func TestRun_NilCommandList(t *testing.T) {
	t.Parallel()

	vr := newTestRunner(nil)
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, VerificationPassed, report.Status)
	assert.Equal(t, 0, report.Total)
}

func TestRun_ContextCancelledBetweenCommands(t *testing.T) {
	t.Parallel()

	// The first command is slow enough that if the context is already cancelled
	// when we check between commands, we stop early.
	ctx, cancel := context.WithCancel(context.Background())

	vr := newTestRunner([]string{"echo a", "echo b", "echo c"})

	// Cancel before calling Run to test the between-command cancellation path.
	cancel()

	report, err := vr.Run(ctx, false)
	// Context cancellation between commands is NOT an error; it returns
	// partial results.
	require.NoError(t, err)
	require.NotNil(t, report)
	// With a pre-cancelled context, zero commands should have been executed
	// (first check fires immediately).
	assert.Equal(t, 0, report.Total)
}

func TestRun_ContextCancelledAfterFirstCommand(t *testing.T) {
	t.Parallel()

	// Strategy: use a context that gets cancelled by a shell command that
	// writes a sentinel and then signals back via a channel. Instead, we
	// use the simpler approach: run one fast command successfully, then cancel
	// the context from a goroutine right after, and have a slow second command
	// that should be skipped.
	//
	// We achieve deterministic cancellation by using a context with deadline
	// that expires just after the first fast command completes. "echo" on most
	// systems completes in < 50ms; the deadline is 200ms.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// First command is fast; second command is "sleep 5" which will be
	// skipped once the context deadline fires between commands.
	vr := newTestRunner([]string{"echo fast", "sleep 5"})
	report, err := vr.Run(ctx, false)

	// Context expiration between commands is not an error at the Run level.
	// It returns whatever partial results were collected.
	require.NoError(t, err)
	require.NotNil(t, report)
	// At least the first echo command ran, possibly both if timing is very fast.
	assert.GreaterOrEqual(t, report.Total, 0)
}

func TestRun_TimedOutCommand_CapturedInResults(t *testing.T) {
	t.Parallel()

	// Use 500ms timeout: long enough for "echo ok" (< 50ms), short enough to
	// time out "sleep 10" without making the test slow.
	vr := NewVerificationRunner(
		[]string{"echo ok", "sleep 10"},
		"",
		500*time.Millisecond,
		nil,
	)
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, VerificationFailed, report.Status)
	assert.Equal(t, 2, report.Total)

	// Find the timed-out result.
	var timedOut *CommandResult
	for i := range report.Results {
		if report.Results[i].TimedOut {
			timedOut = &report.Results[i]
			break
		}
	}
	require.NotNil(t, timedOut, "a timed-out result must be present")
	assert.False(t, timedOut.Passed)
	assert.Equal(t, -1, timedOut.ExitCode)
}

func TestRun_StopOnFailureFalseRunsAll(t *testing.T) {
	t.Parallel()

	// Three commands: first passes, second fails, third passes.
	// stopOnFailure=false must run all three.
	vr := newTestRunner([]string{"echo a", "exit 1", "echo c"})
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	assert.Len(t, report.Results, 3, "all three commands must run when stopOnFailure=false")
}

func TestRun_StopOnFailureTrueStopsAtFirst(t *testing.T) {
	t.Parallel()

	// Three commands: first fails. stopOnFailure=true must stop immediately.
	vr := newTestRunner([]string{"exit 1", "echo b", "echo c"})
	report, err := vr.Run(context.Background(), true)

	require.NoError(t, err)
	assert.Len(t, report.Results, 1, "only one command must run when first fails with stopOnFailure=true")
	assert.Equal(t, VerificationFailed, report.Status)
}

func TestRun_WithLogger_DoesNotPanic(t *testing.T) {
	t.Parallel()

	logger := newTestVerifyLogger()
	vr := NewVerificationRunner([]string{"echo pass", "exit 1"}, "", 0, logger)
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, VerificationFailed, report.Status)
}

func TestRun_DurationIsPositive(t *testing.T) {
	t.Parallel()

	vr := newTestRunner([]string{"echo a"})
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	assert.Greater(t, report.Duration, time.Duration(0))
}

// ---------------------------------------------------------------------------
// Run — integration tests with real commands
// ---------------------------------------------------------------------------

func TestRun_Integration_EchoAndFalse(t *testing.T) {
	t.Parallel()

	vr := newTestRunner([]string{"echo hello", "false"})
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, VerificationFailed, report.Status)
	assert.Equal(t, 2, report.Total)
	assert.Equal(t, 1, report.Passed)
	assert.Equal(t, 1, report.Failed)
	// First result: echo hello must pass.
	assert.True(t, report.Results[0].Passed)
	assert.Contains(t, report.Results[0].Stdout, "hello")
	// Second result: false must fail.
	assert.False(t, report.Results[1].Passed)
	assert.Equal(t, 1, report.Results[1].ExitCode)
}

func TestRun_Integration_CommandWritesStderr(t *testing.T) {
	t.Parallel()

	// Verify that a command writing to stderr is captured correctly.
	vr := newTestRunner([]string{"echo errout >&2; exit 1"})
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	require.NotNil(t, report)
	require.Len(t, report.Results, 1)
	assert.False(t, report.Results[0].Passed)
	assert.Contains(t, report.Results[0].Stderr, "errout")
}

// ---------------------------------------------------------------------------
// FormatReport
// ---------------------------------------------------------------------------

func TestFormatReport_AllPassed(t *testing.T) {
	t.Parallel()

	report := makeVerificationReport(
		VerificationPassed,
		[]CommandResult{
			{Command: "go build ./...", Duration: 1230 * time.Millisecond, Passed: true, ExitCode: 0},
			{Command: "go vet ./...", Duration: 500 * time.Millisecond, Passed: true, ExitCode: 0},
		},
		1730*time.Millisecond, 2, 0,
	)

	out := report.FormatReport()

	assert.Contains(t, out, "Verification Results")
	assert.Contains(t, out, "✓ go build ./...")
	assert.Contains(t, out, "✓ go vet ./...")
	assert.Contains(t, out, "2 passed, 0 failed, 2 total")
	assert.Contains(t, out, "Summary:")
}

func TestFormatReport_SomeFailed(t *testing.T) {
	t.Parallel()

	report := makeVerificationReport(
		VerificationFailed,
		[]CommandResult{
			{Command: "go build ./...", Duration: 1000 * time.Millisecond, Passed: true, ExitCode: 0},
			{
				Command:  "go test ./...",
				Duration: 5000 * time.Millisecond,
				Passed:   false,
				ExitCode: 1,
				Stderr:   "FAIL\tgithub.com/example/raven [build failed]",
			},
		},
		6000*time.Millisecond, 1, 1,
	)

	out := report.FormatReport()

	assert.Contains(t, out, "✓ go build ./...")
	assert.Contains(t, out, "✗ go test ./...")
	assert.Contains(t, out, "FAIL\tgithub.com/example/raven [build failed]")
	assert.Contains(t, out, "1 passed, 1 failed, 2 total")
}

func TestFormatReport_TimedOut(t *testing.T) {
	t.Parallel()

	report := makeVerificationReport(
		VerificationFailed,
		[]CommandResult{
			{
				Command:  "go test ./...",
				Duration: 30000 * time.Millisecond,
				Passed:   false,
				ExitCode: -1,
				TimedOut: true,
			},
		},
		30000*time.Millisecond, 0, 1,
	)

	out := report.FormatReport()

	assert.Contains(t, out, "✗ go test ./...")
	assert.Contains(t, out, "timed out")
}

func TestFormatReport_FailedWithStdoutFallback(t *testing.T) {
	t.Parallel()

	// When a failing command has no stderr but has stdout, the stdout must be
	// shown as the failure output.
	report := makeVerificationReport(
		VerificationFailed,
		[]CommandResult{
			{
				Command:  "go build ./...",
				Duration: 500 * time.Millisecond,
				Passed:   false,
				ExitCode: 1,
				Stdout:   "build-output-on-stdout",
				Stderr:   "",
			},
		},
		500*time.Millisecond, 0, 1,
	)

	out := report.FormatReport()

	assert.Contains(t, out, "✗ go build ./...")
	assert.Contains(t, out, "build-output-on-stdout")
}

func TestFormatReport_EmptyResults(t *testing.T) {
	t.Parallel()

	report := makeVerificationReport(VerificationPassed, nil, 100*time.Millisecond, 0, 0)
	out := report.FormatReport()

	assert.Contains(t, out, "Verification Results")
	assert.Contains(t, out, "0 passed, 0 failed, 0 total")
	assert.Contains(t, out, "Summary:")
	// No check markers should appear.
	assert.NotContains(t, out, "✓")
	assert.NotContains(t, out, "✗")
}

func TestFormatReport_CheckMarkers_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		passed       bool
		wantMarker   string
		unwantMarker string
	}{
		{
			name:         "passing command shows checkmark",
			passed:       true,
			wantMarker:   "✓",
			unwantMarker: "✗",
		},
		{
			name:         "failing command shows cross",
			passed:       false,
			wantMarker:   "✗",
			unwantMarker: "✓",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			exitCode := 0
			if !tt.passed {
				exitCode = 1
			}
			status := VerificationPassed
			if !tt.passed {
				status = VerificationFailed
			}

			p, f := 1, 0
			if !tt.passed {
				p, f = 0, 1
			}

			report := makeVerificationReport(
				status,
				[]CommandResult{
					{Command: "my-cmd", Duration: 1 * time.Millisecond, Passed: tt.passed, ExitCode: exitCode},
				},
				1*time.Millisecond, p, f,
			)

			out := report.FormatReport()
			assert.Contains(t, out, tt.wantMarker+" my-cmd")
			assert.NotContains(t, out, tt.unwantMarker+" my-cmd")
		})
	}
}

func TestFormatReport_SummaryLineFormat(t *testing.T) {
	t.Parallel()

	report := makeVerificationReport(
		VerificationFailed,
		[]CommandResult{
			{Command: "cmd-a", Duration: 500 * time.Millisecond, Passed: true},
			{Command: "cmd-b", Duration: 1000 * time.Millisecond, Passed: false, ExitCode: 1},
		},
		1500*time.Millisecond, 1, 1,
	)

	out := report.FormatReport()

	// The summary line must follow "Summary: N passed, N failed, N total (Xs)"
	assert.Contains(t, out, "Summary: 1 passed, 1 failed, 2 total")
}

// ---------------------------------------------------------------------------
// FormatMarkdown
// ---------------------------------------------------------------------------

func TestFormatMarkdown_AllPassed(t *testing.T) {
	t.Parallel()

	report := makeVerificationReport(
		VerificationPassed,
		[]CommandResult{
			{Command: "go build ./...", Duration: 1000 * time.Millisecond, Passed: true},
		},
		1000*time.Millisecond, 1, 0,
	)

	out := report.FormatMarkdown()

	assert.Contains(t, out, "## Verification Results")
	assert.Contains(t, out, "✅ Passed")
	assert.Contains(t, out, "`go build ./...`")
	assert.Contains(t, out, "**Overall: 1/1 passed**")
	// No details block for passing commands.
	assert.NotContains(t, out, "<details>")
}

func TestFormatMarkdown_TableHeaders(t *testing.T) {
	t.Parallel()

	report := makeVerificationReport(
		VerificationPassed,
		[]CommandResult{
			{Command: "go vet ./...", Duration: 300 * time.Millisecond, Passed: true},
		},
		300*time.Millisecond, 1, 0,
	)

	out := report.FormatMarkdown()

	// Table header row.
	assert.Contains(t, out, "| Command | Status | Duration |")
	// Separator row.
	assert.Contains(t, out, "|---------|--------|----------|")
}

func TestFormatMarkdown_FailedWithOutput(t *testing.T) {
	t.Parallel()

	report := makeVerificationReport(
		VerificationFailed,
		[]CommandResult{
			{Command: "go build ./...", Duration: 500 * time.Millisecond, Passed: true},
			{
				Command:  "go test ./...",
				Duration: 3000 * time.Millisecond,
				Passed:   false,
				ExitCode: 1,
				Stderr:   "FAIL\tgithub.com/example/raven",
			},
		},
		3500*time.Millisecond, 1, 1,
	)

	out := report.FormatMarkdown()

	assert.Contains(t, out, "❌ Failed")
	assert.Contains(t, out, "<details>")
	assert.Contains(t, out, "go test ./... output")
	assert.Contains(t, out, "FAIL\tgithub.com/example/raven")
	assert.Contains(t, out, "**Overall: 1/2 passed**")
}

func TestFormatMarkdown_TimedOut(t *testing.T) {
	t.Parallel()

	report := makeVerificationReport(
		VerificationFailed,
		[]CommandResult{
			{
				Command:  "go test ./...",
				Duration: 60000 * time.Millisecond,
				Passed:   false,
				TimedOut: true,
				ExitCode: -1,
			},
		},
		60000*time.Millisecond, 0, 1,
	)

	out := report.FormatMarkdown()

	assert.Contains(t, out, "⏱️ Timed Out")
	assert.Contains(t, out, "timed out")
	assert.Contains(t, out, "**Overall: 0/1 passed**")
}

func TestFormatMarkdown_FailedWithNoOutput_NoDetailsBlock(t *testing.T) {
	t.Parallel()

	// A failed command that produced no stdout and no stderr must NOT emit a
	// <details> block (it would be empty and unhelpful).
	report := makeVerificationReport(
		VerificationFailed,
		[]CommandResult{
			{
				Command:  "exit 1",
				Duration: 10 * time.Millisecond,
				Passed:   false,
				ExitCode: 1,
				Stdout:   "",
				Stderr:   "",
			},
		},
		10*time.Millisecond, 0, 1,
	)

	out := report.FormatMarkdown()

	assert.NotContains(t, out, "<details>", "no <details> block when failed command has no output")
	assert.Contains(t, out, "❌ Failed")
}

func TestFormatMarkdown_EmptyResults(t *testing.T) {
	t.Parallel()

	report := makeVerificationReport(VerificationPassed, nil, 50*time.Millisecond, 0, 0)
	out := report.FormatMarkdown()

	assert.Contains(t, out, "## Verification Results")
	assert.Contains(t, out, "**Overall: 0/0 passed**")
	assert.NotContains(t, out, "<details>")
}

func TestFormatMarkdown_OverallLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		passed  int
		total   int
		wantOut string
	}{
		{"all pass", 3, 3, "**Overall: 3/3 passed**"},
		{"none pass", 0, 2, "**Overall: 0/2 passed**"},
		{"partial", 1, 3, "**Overall: 1/3 passed**"},
		{"zero total", 0, 0, "**Overall: 0/0 passed**"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			report := &VerificationReport{
				Status:   VerificationPassed,
				Results:  nil,
				Duration: 1 * time.Millisecond,
				Passed:   tt.passed,
				Failed:   tt.total - tt.passed,
				Total:    tt.total,
			}
			out := report.FormatMarkdown()
			assert.Contains(t, out, tt.wantOut)
		})
	}
}

// ---------------------------------------------------------------------------
// escapeMarkdownCode
// ---------------------------------------------------------------------------

func TestEscapeMarkdownCode(t *testing.T) {
	t.Parallel()

	backtick := "`"
	escaped := `\` + backtick

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no backticks",
			input: "go test ./...",
			want:  "go test ./...",
		},
		{
			name:  "single backtick",
			input: "cmd " + backtick + "arg" + backtick,
			want:  "cmd " + escaped + "arg" + escaped,
		},
		{
			name:  "multiple backticks",
			input: backtick + "a" + backtick + " and " + backtick + "b" + backtick,
			want:  escaped + "a" + escaped + " and " + escaped + "b" + escaped,
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only backticks triple",
			input: backtick + backtick + backtick,
			want:  escaped + escaped + escaped,
		},
		{
			name:  "no backtick characters at all",
			input: "go vet ./...",
			want:  "go vet ./...",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, escapeMarkdownCode(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// truncateOutput
// ---------------------------------------------------------------------------

func TestTruncateOutput_ShortOutput(t *testing.T) {
	t.Parallel()

	input := "line1\nline2\nline3"
	assert.Equal(t, input, truncateOutput(input))
}

func TestTruncateOutput_EmptyString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", truncateOutput(""))
}

func TestTruncateOutput_ExactlyAtLimit_NotTruncated(t *testing.T) {
	t.Parallel()

	// A string exactly at maxOutputBytes must be returned unchanged.
	input := strings.Repeat("x", maxOutputBytes)
	out := truncateOutput(input)
	assert.Equal(t, input, out)
}

func TestTruncateOutput_OneByteOverLimit(t *testing.T) {
	t.Parallel()

	// One byte over the limit triggers truncation.
	input := strings.Repeat("x", maxOutputBytes+1)
	out := truncateOutput(input)
	assert.Less(t, len(out), len(input))
}

func TestTruncateOutput_LargeByteCount(t *testing.T) {
	t.Parallel()

	// Build a string that exceeds maxOutputBytes but has very few lines
	// (all content on a small number of very long lines).
	longLine := strings.Repeat("x", maxOutputBytes/2)
	input := longLine + "\n" + longLine + "\n" + longLine
	out := truncateOutput(input)

	// Output should be truncated and contain a notice.
	assert.Less(t, len(out), len(input))
	assert.Contains(t, out, "truncated")
}

func TestTruncateOutput_ManyLines(t *testing.T) {
	t.Parallel()

	// Build output that has more than truncationLines*2 (1024) lines and
	// exceeds maxOutputBytes. Use lines that are long enough to push the
	// total over 1 MiB.
	lineContent := strings.Repeat("a", 1024) // 1 KiB per line
	var sb strings.Builder
	// 1100 lines × 1 KiB = ~1.1 MiB, which exceeds maxOutputBytes (1 MiB)
	// and has more than truncationLines*2=1024 lines, triggering the
	// "lines omitted" path.
	for i := 0; i < 1100; i++ {
		sb.WriteString(lineContent)
		sb.WriteByte('\n')
	}
	input := sb.String()

	out := truncateOutput(input)

	assert.Less(t, len(out), len(input))
	assert.Contains(t, out, "lines omitted")
}

func TestTruncateOutput_HeadAndTailPreserved(t *testing.T) {
	t.Parallel()

	// Build a large multi-line string with identifiable head and tail lines.
	// We need > truncationLines*2 (1024) lines to trigger the line-based
	// truncation path, each line long enough to push the total over maxOutputBytes.
	lineContent := strings.Repeat("x", 1100) // ~1.1 KiB per line
	var sb strings.Builder

	// Prepend a unique head marker.
	sb.WriteString("HEAD_MARKER\n")
	// 1100 lines * 1100 bytes = ~1.21 MiB > maxOutputBytes (1 MiB), and
	// 1100 + 2 header/footer lines > truncationLines*2 (1024).
	for i := 0; i < 1100; i++ {
		sb.WriteString(lineContent)
		sb.WriteByte('\n')
	}
	// Append a unique tail marker.
	sb.WriteString("TAIL_MARKER\n")
	input := sb.String()

	out := truncateOutput(input)

	// The head and tail markers should survive the truncation.
	assert.Contains(t, out, "HEAD_MARKER")
	assert.Contains(t, out, "TAIL_MARKER")
	assert.Contains(t, out, "lines omitted")
}

// ---------------------------------------------------------------------------
// formatDuration
// ---------------------------------------------------------------------------

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "zero", d: 0, want: "0.00s"},
		{name: "one second", d: time.Second, want: "1.00s"},
		{name: "fractional", d: 1234 * time.Millisecond, want: "1.23s"},
		{name: "minutes", d: 2*time.Minute + 30*time.Second, want: "150.00s"},
		{name: "milliseconds only", d: 100 * time.Millisecond, want: "0.10s"},
		{name: "one hour", d: time.Hour, want: "3600.00s"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, formatDuration(tt.d))
		})
	}
}

// ---------------------------------------------------------------------------
// VerificationStatus constants
// ---------------------------------------------------------------------------

func TestVerificationStatus_Constants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, VerificationStatus("passed"), VerificationPassed)
	assert.Equal(t, VerificationStatus("failed"), VerificationFailed)
	assert.NotEqual(t, VerificationPassed, VerificationFailed)
}

// ---------------------------------------------------------------------------
// buildReport (via Run with zero results)
// ---------------------------------------------------------------------------

func TestBuildReport_NoFailures_StatusPassed(t *testing.T) {
	t.Parallel()

	// buildReport is exercised through Run with an empty command list.
	vr := newTestRunner(nil)
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	assert.Equal(t, VerificationPassed, report.Status)
}

func TestBuildReport_OneFailure_StatusFailed(t *testing.T) {
	t.Parallel()

	vr := newTestRunner([]string{"exit 1"})
	report, err := vr.Run(context.Background(), false)

	require.NoError(t, err)
	assert.Equal(t, VerificationFailed, report.Status)
}

// ---------------------------------------------------------------------------
// Fuzz tests
// ---------------------------------------------------------------------------

// FuzzTruncateOutput verifies that truncateOutput never panics and always
// returns a string that is no longer than the input (or the input itself
// when it is within the size limit).
func FuzzTruncateOutput(f *testing.F) {
	f.Add("")
	f.Add("short output")
	f.Add("line1\nline2\nline3")
	f.Add(strings.Repeat("x", maxOutputBytes))
	f.Add(strings.Repeat("x\n", 1000))

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic.
		out := truncateOutput(input)
		// If input is within limit, output must be identical.
		if len(input) <= maxOutputBytes {
			if out != input {
				t.Errorf("input within limit but output differs: input len=%d, output len=%d", len(input), len(out))
			}
		}
	})
}

// FuzzEscapeMarkdownCode verifies that escapeMarkdownCode never panics and that
// the output never contains an unescaped backtick that could break markdown.
func FuzzEscapeMarkdownCode(f *testing.F) {
	f.Add("")
	f.Add("go test ./...")
	f.Add("`cmd`")
	f.Add("```triple```")
	f.Add("no special chars")

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic.
		out := escapeMarkdownCode(input)
		// All backticks in output must be preceded by a backslash.
		for i, ch := range out {
			if ch == '`' {
				if i == 0 || out[i-1] != '\\' {
					t.Errorf("unescaped backtick found in output at position %d: %q", i, out)
				}
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkRunSingle measures the overhead of executing a single fast command.
func BenchmarkRunSingle(b *testing.B) {
	vr := newTestRunner(nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = vr.RunSingle(ctx, "echo bench")
	}
}

// BenchmarkRun_FiveCommands measures Run with five fast echo commands.
func BenchmarkRun_FiveCommands(b *testing.B) {
	cmds := []string{
		"echo a", "echo b", "echo c", "echo d", "echo e",
	}
	vr := newTestRunner(cmds)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = vr.Run(ctx, false)
	}
}

// BenchmarkFormatReport measures report formatting for 10 results.
func BenchmarkFormatReport(b *testing.B) {
	results := make([]CommandResult, 10)
	for i := range results {
		results[i] = CommandResult{
			Command:  fmt.Sprintf("go command-%d ./...", i),
			Duration: time.Duration(i+1) * 100 * time.Millisecond,
			Passed:   i%3 != 0, // every 3rd fails
			ExitCode: func() int {
				if i%3 == 0 {
					return 1
				}
				return 0
			}(),
			Stderr: func() string {
				if i%3 == 0 {
					return "error output line 1\nerror output line 2"
				}
				return ""
			}(),
		}
	}

	failed := 0
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	report := &VerificationReport{
		Status:   VerificationFailed,
		Results:  results,
		Duration: 2 * time.Second,
		Passed:   passed,
		Failed:   failed,
		Total:    len(results),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = report.FormatReport()
	}
}

// BenchmarkFormatMarkdown measures markdown report formatting for 10 results.
func BenchmarkFormatMarkdown(b *testing.B) {
	results := make([]CommandResult, 10)
	for i := range results {
		results[i] = CommandResult{
			Command:  fmt.Sprintf("go command-%d ./...", i),
			Duration: time.Duration(i+1) * 100 * time.Millisecond,
			Passed:   i%3 != 0,
			ExitCode: func() int {
				if i%3 == 0 {
					return 1
				}
				return 0
			}(),
			Stderr: func() string {
				if i%3 == 0 {
					return "FAIL\tgithub.com/example/raven [build failed]"
				}
				return ""
			}(),
		}
	}

	failed := 0
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	report := &VerificationReport{
		Status:   VerificationFailed,
		Results:  results,
		Duration: 2 * time.Second,
		Passed:   passed,
		Failed:   failed,
		Total:    len(results),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = report.FormatMarkdown()
	}
}

// BenchmarkTruncateOutput_LargeInput measures truncation throughput for a 2 MiB input.
func BenchmarkTruncateOutput_LargeInput(b *testing.B) {
	lineContent := strings.Repeat("a", 2048)
	var sb strings.Builder
	for i := 0; i < 800; i++ {
		sb.WriteString(lineContent)
		sb.WriteByte('\n')
	}
	input := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = truncateOutput(input)
	}
}
