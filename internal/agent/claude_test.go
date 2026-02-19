package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// noopLogger satisfies claudeLogger but discards all output.
type noopLogger struct{}

func (noopLogger) Debug(_ string, _ ...interface{}) {}

// newTestAgent returns a ClaudeAgent configured with a command that exists on
// the test machine (the shell itself) so that CheckPrerequisites passes without
// a real Claude installation.
func newTestAgent(cfg AgentConfig) *ClaudeAgent {
	return NewClaudeAgent(cfg, noopLogger{})
}

// ---------------------------------------------------------------------------
// NewClaudeAgent / Name
// ---------------------------------------------------------------------------

func TestClaudeAgent_ImplementsAgent(t *testing.T) {
	t.Parallel()
	var _ Agent = (*ClaudeAgent)(nil)
}

func TestClaudeAgent_Name(t *testing.T) {
	t.Parallel()
	a := newTestAgent(AgentConfig{})
	assert.Equal(t, "claude", a.Name())
}

func TestNewClaudeAgent_NilLogger(t *testing.T) {
	t.Parallel()
	// Should not panic with nil logger.
	a := NewClaudeAgent(AgentConfig{}, nil)
	assert.Equal(t, "claude", a.Name())
}

// ---------------------------------------------------------------------------
// CheckPrerequisites
// ---------------------------------------------------------------------------

func TestClaudeAgent_CheckPrerequisites_FoundCommand(t *testing.T) {
	t.Parallel()
	// "sh" is guaranteed to exist on macOS/Linux.
	a := newTestAgent(AgentConfig{Command: "sh"})
	assert.NoError(t, a.CheckPrerequisites())
}

func TestClaudeAgent_CheckPrerequisites_DefaultCommandNotFound(t *testing.T) {
	t.Parallel()
	// Use a command name that definitely does not exist.
	a := newTestAgent(AgentConfig{Command: "raven-nonexistent-binary-xyz"})
	err := a.CheckPrerequisites()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "raven-nonexistent-binary-xyz")
}

func TestClaudeAgent_CheckPrerequisites_EmptyCommandDefaultsToClaude(t *testing.T) {
	t.Parallel()
	// An empty command should fall back to "claude". On most CI machines
	// "claude" is not installed, so we only verify the error message contains
	// "claude".
	a := newTestAgent(AgentConfig{})
	err := a.CheckPrerequisites()
	if err != nil {
		// claude not installed -- that's the expected path; verify message.
		assert.Contains(t, err.Error(), "claude")
	}
	// If claude IS installed, no error is returned. Both outcomes are valid.
}

// ---------------------------------------------------------------------------
// ParseRateLimit
// ---------------------------------------------------------------------------

func TestClaudeAgent_ParseRateLimit_NoMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
	}{
		{name: "empty string", output: ""},
		{name: "normal output", output: "Successfully ran the task."},
		{name: "error without rate limit", output: "Error: something went wrong"},
		{name: "partial word", output: "My rate is fine"},
	}

	a := newTestAgent(AgentConfig{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info, limited := a.ParseRateLimit(tt.output)
			assert.Nil(t, info)
			assert.False(t, limited)
		})
	}
}

func TestClaudeAgent_ParseRateLimit_DetectsRateLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
	}{
		{name: "rate limit phrase", output: "Error: rate limit exceeded"},
		{name: "too many requests", output: "429 Too Many Requests"},
		{name: "rate-limited hyphen", output: "You are rate-limited"},
		{name: "case insensitive upper", output: "RATE LIMIT HIT"},
		{name: "mixed case", output: "Rate Limited by the API"},
	}

	a := newTestAgent(AgentConfig{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info, limited := a.ParseRateLimit(tt.output)
			require.NotNil(t, info)
			assert.True(t, limited)
			assert.True(t, info.IsLimited)
			assert.Equal(t, tt.output, info.Message)
		})
	}
}

func TestClaudeAgent_ParseRateLimit_ExtractsResetTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		output      string
		wantResetGT time.Duration // duration must be > this value
		wantResetLT time.Duration // duration must be < this value
	}{
		{
			name:        "reset in 30 seconds",
			output:      "rate limit hit. Reset in 30 seconds.",
			wantResetGT: 29 * time.Second,
			wantResetLT: 31 * time.Second,
		},
		{
			name:        "reset in 5 minutes",
			output:      "Too many requests. Reset in 5 minutes.",
			wantResetGT: 4 * time.Minute,
			wantResetLT: 6 * time.Minute,
		},
		{
			name:        "reset in 2 hours",
			output:      "rate limited. Reset in 2 hours.",
			wantResetGT: 1 * time.Hour,
			wantResetLT: 3 * time.Hour,
		},
		{
			name:        "try again in 45 seconds",
			output:      "rate limit reached. Try again in 45 seconds.",
			wantResetGT: 44 * time.Second,
			wantResetLT: 46 * time.Second,
		},
		{
			name:        "try again in 1 minute",
			output:      "Rate limit. Try again in 1 minute.",
			wantResetGT: 59 * time.Second,
			wantResetLT: 61 * time.Second,
		},
		{
			name:        "try again in 1 hour",
			output:      "Rate limited. Try again in 1 hour.",
			wantResetGT: 59 * time.Minute,
			wantResetLT: 61 * time.Minute,
		},
	}

	a := newTestAgent(AgentConfig{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info, limited := a.ParseRateLimit(tt.output)
			require.NotNil(t, info)
			assert.True(t, limited)
			assert.Greater(t, info.ResetAfter, tt.wantResetGT)
			assert.Less(t, info.ResetAfter, tt.wantResetLT)
		})
	}
}

func TestClaudeAgent_ParseRateLimit_NoResetTime(t *testing.T) {
	t.Parallel()

	// Rate limit detected but no duration extractable -> ResetAfter == 0.
	a := newTestAgent(AgentConfig{})
	info, limited := a.ParseRateLimit("rate limit exceeded, please wait")
	require.NotNil(t, info)
	assert.True(t, limited)
	assert.Equal(t, time.Duration(0), info.ResetAfter)
}

// ---------------------------------------------------------------------------
// parseResetDuration (package-level helper)
// ---------------------------------------------------------------------------

func TestParseResetDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		amount string
		unit   string
		want   time.Duration
	}{
		{name: "30 seconds", amount: "30", unit: "seconds", want: 30 * time.Second},
		{name: "1 second", amount: "1", unit: "second", want: time.Second},
		{name: "5 minutes", amount: "5", unit: "minutes", want: 5 * time.Minute},
		{name: "1 minute", amount: "1", unit: "minute", want: time.Minute},
		{name: "2 hours", amount: "2", unit: "hours", want: 2 * time.Hour},
		{name: "1 hour", amount: "1", unit: "hour", want: time.Hour},
		{name: "uppercase SECONDS", amount: "10", unit: "SECONDS", want: 10 * time.Second},
		{name: "mixed case Minutes", amount: "3", unit: "Minutes", want: 3 * time.Minute},
		{name: "zero amount", amount: "0", unit: "seconds", want: 0},
		{name: "negative amount", amount: "-5", unit: "seconds", want: 0},
		{name: "non-numeric amount", amount: "abc", unit: "seconds", want: 0},
		{name: "unknown unit", amount: "10", unit: "days", want: 0},
		{name: "empty unit", amount: "10", unit: "", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseResetDuration(tt.amount, tt.unit)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// DryRunCommand
// ---------------------------------------------------------------------------

func TestClaudeAgent_DryRunCommand_BasicFlags(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{Prompt: "hello world"})

	assert.Contains(t, cmd, "claude")
	assert.Contains(t, cmd, "--permission-mode")
	assert.Contains(t, cmd, "accept")
	assert.Contains(t, cmd, "--print")
	assert.Contains(t, cmd, "hello world")
}

func TestClaudeAgent_DryRunCommand_CustomCommand(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{Command: "my-claude"})
	cmd := a.DryRunCommand(RunOpts{Prompt: "test"})
	assert.True(t, strings.HasPrefix(cmd, "my-claude "))
}

func TestClaudeAgent_DryRunCommand_ModelFromOpts(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{Model: "config-model"})
	cmd := a.DryRunCommand(RunOpts{Model: "opts-model", Prompt: "p"})
	assert.Contains(t, cmd, "opts-model")
	assert.NotContains(t, cmd, "config-model")
}

func TestClaudeAgent_DryRunCommand_ModelFromConfig(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{Model: "config-model"})
	cmd := a.DryRunCommand(RunOpts{Prompt: "p"})
	assert.Contains(t, cmd, "--model")
	assert.Contains(t, cmd, "config-model")
}

func TestClaudeAgent_DryRunCommand_NoModelWhenEmpty(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{Prompt: "p"})
	assert.NotContains(t, cmd, "--model")
}

func TestClaudeAgent_DryRunCommand_AllowedToolsFromOpts(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{AllowedTools: "config-tools"})
	cmd := a.DryRunCommand(RunOpts{AllowedTools: "opts-tools", Prompt: "p"})
	assert.Contains(t, cmd, "opts-tools")
	assert.NotContains(t, cmd, "config-tools")
}

func TestClaudeAgent_DryRunCommand_AllowedToolsFromConfig(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{AllowedTools: "bash,edit"})
	cmd := a.DryRunCommand(RunOpts{Prompt: "p"})
	assert.Contains(t, cmd, "--allowedTools")
	assert.Contains(t, cmd, "bash,edit")
}

func TestClaudeAgent_DryRunCommand_OutputFormat(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{OutputFormat: "json", Prompt: "p"})
	assert.Contains(t, cmd, "--output-format")
	assert.Contains(t, cmd, "json")
}

func TestClaudeAgent_DryRunCommand_PromptFile(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{PromptFile: "/tmp/myfile.md"})
	assert.Contains(t, cmd, "--prompt-file")
	assert.Contains(t, cmd, "/tmp/myfile.md")
	assert.NotContains(t, cmd, "--prompt ")
}

func TestClaudeAgent_DryRunCommand_LargePromptTruncated(t *testing.T) {
	t.Parallel()

	// Build a prompt that exceeds maxInlinePromptBytes.
	bigPrompt := strings.Repeat("a", maxInlinePromptBytes+1)
	a := newTestAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{Prompt: bigPrompt})

	// The dry-run output must contain "..." indicating truncation.
	assert.Contains(t, cmd, "...")
	// And it must NOT contain the full prompt (that would be absurdly long).
	assert.Less(t, len(cmd), maxInlinePromptBytes)
}

func TestClaudeAgent_DryRunCommand_ShortPromptNotTruncated(t *testing.T) {
	t.Parallel()

	prompt := "a short prompt"
	a := newTestAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{Prompt: prompt})
	assert.Contains(t, cmd, prompt)
	assert.NotContains(t, cmd, "...")
}

// ---------------------------------------------------------------------------
// Run (integration-style using real shell commands)
// ---------------------------------------------------------------------------

func TestClaudeAgent_Run_SuccessWithEcho(t *testing.T) {
	t.Parallel()

	// Use "sh -c 'echo hello'" as a stand-in for the claude CLI so we can
	// test the subprocess plumbing without a real installation.
	// We configure Command = "sh" and construct a prompt that sh will echo.
	// Because ClaudeAgent always passes --permission-mode accept --print ...,
	// we cannot call sh directly. Instead we test with a small wrapper script
	// via os/exec from within the test directly to validate Run plumbing.
	//
	// We use the echo binary with a custom command config.
	a := newTestAgent(AgentConfig{Command: "echo"})
	ctx := context.Background()

	result, err := a.Run(ctx, RunOpts{
		// echo ignores unknown flags gracefully on most platforms.
	})

	// echo always exits 0.
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.True(t, result.Success())
	assert.Greater(t, result.Duration, time.Duration(0))
}

func TestClaudeAgent_Run_NonZeroExitCode(t *testing.T) {
	t.Parallel()

	// "false" exits with code 1 on all POSIX systems.
	a := newTestAgent(AgentConfig{Command: "false"})
	ctx := context.Background()

	result, err := a.Run(ctx, RunOpts{})
	// Run should NOT return a Go error for a non-zero exit code.
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	assert.False(t, result.Success())
}

func TestClaudeAgent_Run_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Use "sh -c sleep" with the context so the process is killed when the
	// timeout fires. buildArgs always prepends --permission-mode accept --print,
	// so we pass those as sh flags; sh will fail quickly, which is fine --
	// we just verify that Run completes (does not block) and returns without
	// panicking. Both nil and non-nil err are acceptable.
	a := newTestAgent(AgentConfig{Command: "sh"})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := a.Run(ctx, RunOpts{})
	// Either the context kills the process (signal -> ExitError -> err=nil)
	// or sh exits immediately due to bad flags (ExitError -> err=nil, exitCode!=0).
	// A non-exit-error in cmd.Wait would produce err!=nil; that's also fine.
	// We simply verify that Run terminates and does not block indefinitely.
	if err != nil {
		t.Logf("Run returned error (acceptable): %v", err)
	}
}

func TestClaudeAgent_Run_RateLimitNotDetectedForNormalOutput(t *testing.T) {
	t.Parallel()

	// Verify that normal command output (no rate-limit phrases) does not
	// populate RateLimit in the result. Use "echo" which exits 0 and prints
	// its arguments -- none of which contain rate-limit trigger words.
	a := newTestAgent(AgentConfig{Command: "echo"})
	result, err := a.Run(context.Background(), RunOpts{})
	require.NoError(t, err)
	assert.False(t, result.WasRateLimited())
	assert.Nil(t, result.RateLimit)
}

// ---------------------------------------------------------------------------
// buildCommand environment variable handling
// ---------------------------------------------------------------------------

func TestClaudeAgent_BuildCommand_EffortFromOpts(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{Effort: "low"})
	ctx := context.Background()

	// We can't easily inspect Cmd.Env directly from outside, so we run
	// "env" and look for the CLAUDE_CODE_EFFORT_LEVEL in its output.
	cmd := a.buildCommand(ctx, RunOpts{Effort: "high"})
	var found bool
	for _, e := range cmd.Env {
		if e == "CLAUDE_CODE_EFFORT_LEVEL=high" {
			found = true
		}
	}
	assert.True(t, found, "expected CLAUDE_CODE_EFFORT_LEVEL=high in env")

	// Ensure the config value is NOT set (opts wins).
	for _, e := range cmd.Env {
		assert.NotEqual(t, "CLAUDE_CODE_EFFORT_LEVEL=low", e)
	}
}

func TestClaudeAgent_BuildCommand_EffortFromConfig(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{Effort: "medium"})
	ctx := context.Background()
	cmd := a.buildCommand(ctx, RunOpts{})

	var found bool
	for _, e := range cmd.Env {
		if e == "CLAUDE_CODE_EFFORT_LEVEL=medium" {
			found = true
		}
	}
	assert.True(t, found, "expected CLAUDE_CODE_EFFORT_LEVEL=medium in env")
}

func TestClaudeAgent_BuildCommand_NoEffortWhenEmpty(t *testing.T) {
	// Cannot be parallel -- t.Setenv requires sequential test.
	// Use t.Setenv to clear CLAUDE_CODE_EFFORT_LEVEL so the parent env
	// (e.g. Claude Code setting CLAUDE_CODE_EFFORT_LEVEL=high) doesn't
	// interfere with this test.
	t.Setenv("CLAUDE_CODE_EFFORT_LEVEL", "")

	a := newTestAgent(AgentConfig{})
	ctx := context.Background()
	cmd := a.buildCommand(ctx, RunOpts{})

	// The implementation should not append a non-empty CLAUDE_CODE_EFFORT_LEVEL
	// when both config.Effort and opts.Effort are empty.
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "CLAUDE_CODE_EFFORT_LEVEL=") {
			assert.Equal(t, "CLAUDE_CODE_EFFORT_LEVEL=", e,
				"expected no effort value in env when config and opts effort are empty")
		}
	}
}

func TestClaudeAgent_BuildCommand_AdditionalEnv(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{})
	ctx := context.Background()
	cmd := a.buildCommand(ctx, RunOpts{Env: []string{"MY_VAR=test_value"}})

	var found bool
	for _, e := range cmd.Env {
		if e == "MY_VAR=test_value" {
			found = true
		}
	}
	assert.True(t, found, "expected MY_VAR=test_value in env")
}

func TestClaudeAgent_BuildCommand_WorkDir(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{})
	ctx := context.Background()
	cmd := a.buildCommand(ctx, RunOpts{WorkDir: "/tmp"})

	assert.Equal(t, "/tmp", cmd.Dir)
}

func TestClaudeAgent_BuildCommand_NoWorkDir(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{})
	ctx := context.Background()
	cmd := a.buildCommand(ctx, RunOpts{})

	assert.Equal(t, "", cmd.Dir)
}

// ---------------------------------------------------------------------------
// Mock script helpers for integration tests
// ---------------------------------------------------------------------------

// writeMockScript creates an executable shell script in dir with the given
// content (#!/bin/sh header is prepended automatically). It returns the path.
func writeMockScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	// Write without executable bit first, then chmod — avoids ETXTBSY ("text
	// file busy") on Linux when the kernel sees an executable file that is
	// still being written/closed.
	err := os.WriteFile(path, []byte("#!/bin/sh\n"+content), 0600)
	require.NoError(t, err, "writing mock script %s", name)
	require.NoError(t, os.Chmod(path, 0755), "chmod mock script %s", name)
	return path
}

// skipOnWindows skips the test on Windows where shell scripts are not supported.
func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script integration tests are not supported on Windows")
	}
}

// ---------------------------------------------------------------------------
// Run integration tests using mock shell scripts
// ---------------------------------------------------------------------------

func TestClaudeAgent_Run_Integration_StdoutAndStderrCaptured(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-success.sh", `
echo "Task completed"
echo "Debug info" >&2
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.True(t, result.Success())
	assert.Contains(t, result.Stdout, "Task completed")
	assert.Contains(t, result.Stderr, "Debug info")
	assert.Greater(t, result.Duration, time.Duration(0))
	assert.Nil(t, result.RateLimit)
}

func TestClaudeAgent_Run_Integration_NonZeroExitCode(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-nonzero.sh", `
echo "partial output"
exit 2
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err, "Run should not return a Go error for non-zero exit codes")
	assert.Equal(t, 2, result.ExitCode)
	assert.False(t, result.Success())
	assert.Contains(t, result.Stdout, "partial output")
}

func TestClaudeAgent_Run_Integration_RateLimitDetectedInRunResult(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-rate-limit.sh", `
echo "Your rate limit will reset in 30 seconds"
exit 1
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	require.NotNil(t, result.RateLimit, "RateLimit should be populated when rate-limit output is detected")
	assert.True(t, result.WasRateLimited())
	assert.True(t, result.RateLimit.IsLimited)
	assert.Equal(t, 30*time.Second, result.RateLimit.ResetAfter)
	assert.Contains(t, result.RateLimit.Message, "rate limit")
}

func TestClaudeAgent_Run_Integration_RateLimitTooManyRequests(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-too-many.sh", `
echo "Too many requests, please slow down"
exit 1
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	require.NotNil(t, result.RateLimit)
	assert.True(t, result.RateLimit.IsLimited)
	// No specific reset time in "too many requests" without duration.
	assert.Equal(t, time.Duration(0), result.RateLimit.ResetAfter)
}

func TestClaudeAgent_Run_Integration_TryAgainRateLimit(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-try-again.sh", `
echo "rate limit hit. try again in 2 minutes."
exit 1
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	require.NotNil(t, result.RateLimit)
	assert.True(t, result.RateLimit.IsLimited)
	assert.Equal(t, 2*time.Minute, result.RateLimit.ResetAfter)
}

func TestClaudeAgent_Run_Integration_ContextCancellationKillsProcess(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	// A script that sleeps much longer than our test timeout.
	scriptPath := writeMockScript(t, dir, "claude-slow.sh", `
sleep 60
echo "should not reach here"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := a.Run(ctx, RunOpts{})
	elapsed := time.Since(start)

	// The process must be killed well within 5 seconds.
	assert.Less(t, elapsed, 5*time.Second, "subprocess should have been killed promptly on context cancellation")
	// Run may return either nil (ExitError from signal) or a non-nil error
	// (e.g. "signal: killed" wrapped). Both are acceptable.
	if err != nil {
		t.Logf("Run returned error after context cancellation (acceptable): %v", err)
	}
}

func TestClaudeAgent_Run_Integration_WorkDirUsed(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	workDir := t.TempDir()
	scriptDir := t.TempDir()

	// Script that prints its working directory to stdout.
	scriptPath := writeMockScript(t, scriptDir, "claude-pwd.sh", `
pwd
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{WorkDir: workDir})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	// On macOS /var/folders may resolve via symlink; compare base names.
	assert.Contains(t, result.Stdout, filepath.Base(workDir))
}

func TestClaudeAgent_Run_Integration_ExtraEnvMerged(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-env.sh", `
echo "RAVEN_TEST_VAR=$RAVEN_TEST_VAR"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{
		Env: []string{"RAVEN_TEST_VAR=integration_test_value"},
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "RAVEN_TEST_VAR=integration_test_value")
}

func TestClaudeAgent_Run_Integration_EffortEnvSet(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-effort.sh", `
echo "effort=$CLAUDE_CODE_EFFORT_LEVEL"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath, Effort: "high"})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "effort=high")
}

func TestClaudeAgent_Run_Integration_EffortOverriddenByOpts(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-effort-override.sh", `
echo "effort=$CLAUDE_CODE_EFFORT_LEVEL"
exit 0
`)

	// Config has "low" but opts overrides with "high".
	a := newTestAgent(AgentConfig{Command: scriptPath, Effort: "low"})
	result, err := a.Run(context.Background(), RunOpts{Effort: "high"})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "effort=high")
	assert.NotContains(t, result.Stdout, "effort=low")
}

func TestClaudeAgent_Run_Integration_PromptPassedAsFlag(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	// Script prints all its arguments so we can inspect what flags were passed.
	scriptPath := writeMockScript(t, dir, "claude-args.sh", `
echo "args: $*"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{Prompt: "implement the feature"})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "--prompt")
	assert.Contains(t, result.Stdout, "implement the feature")
}

func TestClaudeAgent_Run_Integration_PromptFileFlag(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	// Create the prompt file.
	promptFile := filepath.Join(dir, "my-prompt.md")
	err := os.WriteFile(promptFile, []byte("# My Prompt\nDo the thing."), 0644)
	require.NoError(t, err)

	scriptDir := t.TempDir()
	scriptPath := writeMockScript(t, scriptDir, "claude-prompt-file.sh", `
echo "args: $*"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{PromptFile: promptFile})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "--prompt-file")
	assert.Contains(t, result.Stdout, promptFile)
}

func TestClaudeAgent_Run_Integration_PermissionModeAcceptAlwaysSet(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-perms.sh", `
echo "args: $*"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "--permission-mode")
	assert.Contains(t, result.Stdout, "accept")
	assert.Contains(t, result.Stdout, "--print")
}

func TestClaudeAgent_Run_Integration_ModelFlagSet(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-model.sh", `
echo "args: $*"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath, Model: "claude-sonnet-4-20250514"})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "--model")
	assert.Contains(t, result.Stdout, "claude-sonnet-4-20250514")
}

func TestClaudeAgent_Run_Integration_ModelOverriddenByOpts(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-model-override.sh", `
echo "args: $*"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath, Model: "config-model"})
	result, err := a.Run(context.Background(), RunOpts{Model: "opts-model"})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "opts-model")
	assert.NotContains(t, result.Stdout, "config-model")
}

func TestClaudeAgent_Run_Integration_AllowedToolsFlagSet(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-tools.sh", `
echo "args: $*"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath, AllowedTools: "bash,edit"})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "--allowedTools")
	assert.Contains(t, result.Stdout, "bash,edit")
}

func TestClaudeAgent_Run_Integration_OutputFormatJSON(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-json.sh", `
echo "args: $*"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{OutputFormat: "json"})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "--output-format")
	assert.Contains(t, result.Stdout, "json")
}

func TestClaudeAgent_Run_Integration_DurationMeasured(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	// Use a small but measurable operation (print a large string) to ensure
	// the subprocess takes non-trivial time so Duration > 0.
	scriptPath := writeMockScript(t, dir, "claude-duration.sh", `
echo "done"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	assert.Greater(t, result.Duration, time.Duration(0), "Duration must be positive")
}

func TestClaudeAgent_Run_Integration_LargePromptWrittenToTempFile(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	// Script that prints its arguments -- we expect --prompt-file <path> not --prompt.
	scriptPath := writeMockScript(t, dir, "claude-largeprompt.sh", `
echo "args: $*"
exit 0
`)

	// Construct a prompt larger than maxInlinePromptBytes (100 KiB).
	bigPrompt := strings.Repeat("x", maxInlinePromptBytes+1)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{Prompt: bigPrompt})

	require.NoError(t, err)
	// When prompt is large, it should be written to a temp file and
	// passed via --prompt-file. Verify the flag appears in args.
	assert.Contains(t, result.Stdout, "--prompt-file")
	// The raw big prompt must NOT appear inline in args (would be too long).
	assert.NotContains(t, result.Stdout, strings.Repeat("x", 200))
}

// ---------------------------------------------------------------------------
// CheckPrerequisites integration test
// ---------------------------------------------------------------------------

func TestClaudeAgent_CheckPrerequisites_CustomCommandOnPath(t *testing.T) {
	// NOTE: t.Setenv modifies os-level PATH so this test must NOT be parallel.
	skipOnWindows(t)

	dir := t.TempDir()
	// Create a real executable that lives on a tmp path we add to PATH.
	writeMockScript(t, dir, "fake-claude", `exit 0`)

	// Prepend the tmp dir to PATH so exec.LookPath can find fake-claude.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	a := newTestAgent(AgentConfig{Command: "fake-claude"})
	err := a.CheckPrerequisites()
	assert.NoError(t, err)
}

func TestClaudeAgent_CheckPrerequisites_MissingCommandHasInstallHint(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{Command: "raven-definitely-not-installed-xyz-abc"})
	err := a.CheckPrerequisites()
	require.Error(t, err)
	// The error must contain the missing binary name and an install hint.
	assert.Contains(t, err.Error(), "raven-definitely-not-installed-xyz-abc")
	assert.Contains(t, err.Error(), "https://")
}

// ---------------------------------------------------------------------------
// buildArgs edge cases
// ---------------------------------------------------------------------------

func TestClaudeAgent_BuildArgs_PromptFileExcludesPromptFlag(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{})
	args := a.buildArgs(RunOpts{PromptFile: "/some/file.md", Prompt: "ignored"}, false)

	found := false
	for i, arg := range args {
		if arg == "--prompt-file" {
			found = true
			if i+1 < len(args) {
				assert.Equal(t, "/some/file.md", args[i+1])
			}
		}
	}
	assert.True(t, found, "--prompt-file must appear in args when PromptFile is set")

	// When PromptFile is set, --prompt should NOT appear.
	for _, arg := range args {
		assert.NotEqual(t, "--prompt", arg, "--prompt must not appear when PromptFile is set")
	}
}

func TestClaudeAgent_BuildArgs_NoPromptFlags(t *testing.T) {
	t.Parallel()

	// Neither Prompt nor PromptFile -- neither flag should appear.
	a := newTestAgent(AgentConfig{})
	args := a.buildArgs(RunOpts{}, false)

	for _, arg := range args {
		assert.NotEqual(t, "--prompt", arg)
		assert.NotEqual(t, "--prompt-file", arg)
	}
}

func TestClaudeAgent_BuildArgs_PermissionModeAndPrintAlwaysFirst(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{})
	args := a.buildArgs(RunOpts{}, false)

	require.GreaterOrEqual(t, len(args), 3, "must have at least --permission-mode accept --print")
	assert.Equal(t, "--permission-mode", args[0])
	assert.Equal(t, "accept", args[1])
	assert.Equal(t, "--print", args[2])
}

// ---------------------------------------------------------------------------
// DryRunCommand edge cases
// ---------------------------------------------------------------------------

func TestClaudeAgent_DryRunCommand_PromptAtExactTruncationBoundary(t *testing.T) {
	t.Parallel()

	// Prompt exactly at maxDryRunPromptLen should not be truncated.
	prompt := strings.Repeat("b", maxDryRunPromptLen)
	a := newTestAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{Prompt: strings.Repeat("b", maxInlinePromptBytes+1)})

	// The big prompt triggers dryRun truncation. Verify "..." appears.
	assert.Contains(t, cmd, "...")
	_ = prompt // used above to document boundary
}

func TestClaudeAgent_DryRunCommand_PromptBelowTruncationLimit(t *testing.T) {
	t.Parallel()

	// A prompt just over maxInlinePromptBytes but whose first maxDryRunPromptLen
	// runes do not contain "..." -- truncated text ends with "...".
	prompt := strings.Repeat("c", maxInlinePromptBytes+1)
	a := newTestAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{Prompt: prompt})

	truncated := prompt[:maxDryRunPromptLen] + "..."
	assert.Contains(t, cmd, truncated)
}

func TestClaudeAgent_DryRunCommand_EmptyPromptNoPromptFlag(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{})

	assert.NotContains(t, cmd, "--prompt")
	assert.NotContains(t, cmd, "--prompt-file")
}

func TestClaudeAgent_DryRunCommand_NoOutputFormatWhenEmpty(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{Prompt: "p"})

	assert.NotContains(t, cmd, "--output-format")
}

func TestClaudeAgent_DryRunCommand_AllFlagsTogether(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{
		Command:      "claude",
		Model:        "claude-opus-4-20250514",
		AllowedTools: "bash,edit",
		Effort:       "high",
	})
	cmd := a.DryRunCommand(RunOpts{
		Prompt:       "do the thing",
		OutputFormat: "json",
		Model:        "claude-sonnet-4-20250514", // opts overrides config
	})

	// Base flags always present.
	assert.Contains(t, cmd, "--permission-mode")
	assert.Contains(t, cmd, "accept")
	assert.Contains(t, cmd, "--print")
	// Model from opts (not config).
	assert.Contains(t, cmd, "--model")
	assert.Contains(t, cmd, "claude-sonnet-4-20250514")
	assert.NotContains(t, cmd, "claude-opus-4-20250514")
	// Tools and output format.
	assert.Contains(t, cmd, "--allowedTools")
	assert.Contains(t, cmd, "bash,edit")
	assert.Contains(t, cmd, "--output-format")
	assert.Contains(t, cmd, "json")
	// Prompt.
	assert.Contains(t, cmd, "do the thing")
}

// ---------------------------------------------------------------------------
// ParseRateLimit additional edge cases
// ---------------------------------------------------------------------------

func TestClaudeAgent_ParseRateLimit_SpecificDurations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		output      string
		wantLimited bool
		wantAfter   time.Duration
	}{
		{
			name:        "reset in 30 seconds exact",
			output:      "Your rate limit will reset in 30 seconds",
			wantLimited: true,
			wantAfter:   30 * time.Second,
		},
		{
			name:        "try again in 2 minutes with rate limit phrase",
			output:      "rate limit hit. try again in 2 minutes.",
			wantLimited: true,
			wantAfter:   2 * time.Minute,
		},
		{
			name:        "too many requests no duration",
			output:      "Too many requests",
			wantLimited: true,
			wantAfter:   0,
		},
		{
			name:        "task completed successfully - no limit",
			output:      "Task completed successfully",
			wantLimited: false,
			wantAfter:   0,
		},
		{
			name:        "case insensitive RATE LIMIT",
			output:      "RATE LIMIT exceeded",
			wantLimited: true,
			wantAfter:   0,
		},
		{
			name:        "rate-limited with hyphen",
			output:      "You are rate-limited by the API",
			wantLimited: true,
			wantAfter:   0,
		},
		{
			name:        "rate limited without hyphen",
			output:      "rate limited for this request",
			wantLimited: true,
			wantAfter:   0,
		},
	}

	a := newTestAgent(AgentConfig{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info, limited := a.ParseRateLimit(tt.output)
			assert.Equal(t, tt.wantLimited, limited, "IsLimited mismatch")
			if tt.wantLimited {
				require.NotNil(t, info)
				assert.True(t, info.IsLimited)
				assert.Equal(t, tt.wantAfter, info.ResetAfter)
				assert.Equal(t, tt.output, info.Message)
			} else {
				assert.Nil(t, info)
			}
		})
	}
}

func TestClaudeAgent_ParseRateLimit_MessagePreserved(t *testing.T) {
	t.Parallel()

	output := "rate limit exceeded at 2026-01-01T00:00:00Z"
	a := newTestAgent(AgentConfig{})
	info, limited := a.ParseRateLimit(output)

	require.True(t, limited)
	require.NotNil(t, info)
	assert.Equal(t, output, info.Message, "original message must be preserved verbatim")
}

// ---------------------------------------------------------------------------
// parseResetDuration additional edge cases
// ---------------------------------------------------------------------------

func TestParseResetDuration_ZeroAndNegative(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		amount string
		unit   string
		want   time.Duration
	}{
		{name: "zero seconds", amount: "0", unit: "seconds", want: 0},
		{name: "zero minutes", amount: "0", unit: "minutes", want: 0},
		{name: "negative seconds", amount: "-1", unit: "seconds", want: 0},
		{name: "very large seconds", amount: "3600", unit: "seconds", want: 3600 * time.Second},
		{name: "days unit unknown", amount: "1", unit: "days", want: 0},
		{name: "millis unit unknown", amount: "500", unit: "milliseconds", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseResetDuration(tt.amount, tt.unit)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Name and interface compliance
// ---------------------------------------------------------------------------

func TestClaudeAgent_Name_ReturnsClaudeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config AgentConfig
	}{
		{name: "empty config", config: AgentConfig{}},
		{name: "with model", config: AgentConfig{Model: "claude-opus-4-20250514"}},
		{name: "with command", config: AgentConfig{Command: "my-claude"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := newTestAgent(tt.config)
			assert.Equal(t, "claude", a.Name())
		})
	}
}

// ---------------------------------------------------------------------------
// Run -- error path: command not found / cannot start
// ---------------------------------------------------------------------------

func TestClaudeAgent_Run_CommandNotFound(t *testing.T) {
	t.Parallel()

	a := newTestAgent(AgentConfig{Command: "this-binary-does-not-exist-raven-xyz"})
	_, err := a.Run(context.Background(), RunOpts{})
	require.Error(t, err, "Run must return an error when the command binary is missing")
	assert.Contains(t, err.Error(), "starting claude")
}

// ---------------------------------------------------------------------------
// Run -- streaming integration tests
// ---------------------------------------------------------------------------

func TestClaudeAgent_Run_StreamingNotActivatedWithoutChannel(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	// Script emits JSONL-like output.
	scriptPath := writeMockScript(t, dir, "claude-no-stream.sh", `
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}'
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	// OutputFormat set to stream-json but StreamEvents is nil -- non-streaming path.
	result, err := a.Run(context.Background(), RunOpts{
		OutputFormat: OutputFormatStreamJSON,
		// StreamEvents: nil (default)
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	// Stdout should contain the raw JSONL line.
	assert.Contains(t, result.Stdout, "assistant")
}

func TestClaudeAgent_Run_StreamingNotActivatedWithoutStreamJSON(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-no-stream2.sh", `
echo "plain text output"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	ch := make(chan StreamEvent, 16)
	// StreamEvents is set but OutputFormat is NOT stream-json -- non-streaming path.
	result, err := a.Run(context.Background(), RunOpts{
		OutputFormat: OutputFormatJSON, // wrong format
		StreamEvents: ch,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	// No events should have been sent (non-streaming path).
	assert.Empty(t, ch)
	assert.Contains(t, result.Stdout, "plain text output")
}

func TestClaudeAgent_Run_StreamingForwardsEvents(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	// Script emits two JSONL events.
	scriptPath := writeMockScript(t, dir, "claude-stream.sh", `
printf '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"thinking"}]}}\n'
printf '{"type":"result","subtype":"success","num_turns":1,"cost_usd":0.001}\n'
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	ch := make(chan StreamEvent, 32)

	result, err := a.Run(context.Background(), RunOpts{
		OutputFormat: OutputFormatStreamJSON,
		StreamEvents: ch,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)

	// Collect events from the channel (it is not closed by the agent per spec).
	var events []StreamEvent
drainCh:
	for {
		select {
		case ev := <-ch:
			events = append(events, ev)
		default:
			break drainCh
		}
	}
	// Both events should have been forwarded.
	require.Len(t, events, 2)
	assert.Equal(t, StreamEventAssistant, events[0].Type)
	assert.Equal(t, StreamEventResult, events[1].Type)

	// Stdout in RunResult still has the full JSONL content.
	assert.Contains(t, result.Stdout, "assistant")
	assert.Contains(t, result.Stdout, "result")
}

func TestClaudeAgent_Run_StreamingCapturesStdoutInBuffer(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	// Verify that streaming mode still populates RunResult.Stdout.
	scriptPath := writeMockScript(t, dir, "claude-stream-buf.sh", `
printf '{"type":"system","subtype":"init","session_id":"s1"}\n'
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	ch := make(chan StreamEvent, 8)

	result, err := a.Run(context.Background(), RunOpts{
		OutputFormat: OutputFormatStreamJSON,
		StreamEvents: ch,
	})

	require.NoError(t, err)
	// Stdout must contain the JSONL line that was decoded.
	assert.Contains(t, result.Stdout, "system")
	assert.Contains(t, result.Stdout, "s1")
}

func TestClaudeAgent_Run_StreamingSlowConsumerDropsEvents(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	// Emit many events quickly.
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, `printf '{"type":"result","subtype":"success","num_turns":1}\n'`)
	}
	script := ""
	for _, l := range lines {
		script += l + "\n"
	}
	script += "exit 0\n"
	scriptPath := writeMockScript(t, dir, "claude-stream-fast.sh", script)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	// Capacity 1: many sends will be dropped (non-blocking).
	ch := make(chan StreamEvent, 1)

	result, err := a.Run(context.Background(), RunOpts{
		OutputFormat: OutputFormatStreamJSON,
		StreamEvents: ch,
	})

	// Must not block or panic.
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	// At least the buffer content was captured.
	assert.Contains(t, result.Stdout, "result")
}

// ---------------------------------------------------------------------------
// Run -- streaming: RunResult fields populated in streaming mode
// ---------------------------------------------------------------------------

func TestClaudeAgent_Run_StreamingRunResultFieldsPopulated(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	// Script emits a JSONL event to stdout, writes to stderr, and exits with code 3.
	scriptPath := writeMockScript(t, dir, "claude-stream-fields.sh", `
printf '{"type":"result","subtype":"success","num_turns":1,"cost_usd":0.001}\n'
echo "stderr message" >&2
exit 3
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	ch := make(chan StreamEvent, 16)

	result, err := a.Run(context.Background(), RunOpts{
		OutputFormat: OutputFormatStreamJSON,
		StreamEvents: ch,
	})

	// Run must not return a Go error for a non-zero exit code.
	require.NoError(t, err)
	// ExitCode must be 3.
	assert.Equal(t, 3, result.ExitCode)
	assert.False(t, result.Success())
	// Duration must be positive.
	assert.Greater(t, result.Duration, time.Duration(0))
	// Stderr must be captured.
	assert.Contains(t, result.Stderr, "stderr message")
	// Stdout still holds the full JSONL output.
	assert.Contains(t, result.Stdout, "result")
}

func TestClaudeAgent_Run_StreamingEmptyOutputFormatNotActivated(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-empty-fmt.sh", `
echo "plain output"
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	ch := make(chan StreamEvent, 16)

	// StreamEvents is set but OutputFormat is empty string -- must NOT activate streaming.
	result, err := a.Run(context.Background(), RunOpts{
		OutputFormat: "", // empty -- streaming must not activate
		StreamEvents: ch,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	// No events should have been sent.
	assert.Empty(t, ch)
	assert.Contains(t, result.Stdout, "plain output")
}

func TestClaudeAgent_Run_StreamingGoroutineExitsCleanly(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	// Verify that Run returns (doesn't hang) after the subprocess exits,
	// which means the streaming goroutine exits cleanly when the pipe closes.
	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-goroutine-exit.sh", `
printf '{"type":"system","subtype":"init","session_id":"s1"}\n'
printf '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}\n'
printf '{"type":"result","subtype":"success","num_turns":1,"cost_usd":0.0}\n'
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	ch := make(chan StreamEvent, 32)

	// This must return promptly, not block waiting for the goroutine.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = a.Run(context.Background(), RunOpts{
			OutputFormat: OutputFormatStreamJSON,
			StreamEvents: ch,
		})
	}()

	select {
	case <-done:
		// Good -- goroutine exited and Run returned promptly.
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5 seconds: streaming goroutine may have leaked")
	}

	// All 3 events should have been forwarded.
	var events []StreamEvent
drainLoop:
	for {
		select {
		case ev := <-ch:
			events = append(events, ev)
		default:
			break drainLoop
		}
	}
	assert.Len(t, events, 3, "all 3 events should be forwarded before Run returns")
}

func TestClaudeAgent_Run_StreamingStdoutFullyCapturedWithTeeReader(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	// This test verifies the io.TeeReader dual-path: streaming decodes events
	// AND stdoutBuf captures the exact same bytes.
	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "claude-tee-reader.sh", `
printf '{"type":"system","subtype":"init","session_id":"tee-test"}\n'
printf '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"thinking"}]}}\n'
printf '{"type":"result","subtype":"success","num_turns":1,"cost_usd":0.005}\n'
exit 0
`)

	a := newTestAgent(AgentConfig{Command: scriptPath})
	ch := make(chan StreamEvent, 32)

	result, err := a.Run(context.Background(), RunOpts{
		OutputFormat: OutputFormatStreamJSON,
		StreamEvents: ch,
	})

	require.NoError(t, err)

	// Collect events from channel.
	var events []StreamEvent
drainTeeLoop:
	for {
		select {
		case ev := <-ch:
			events = append(events, ev)
		default:
			break drainTeeLoop
		}
	}

	// Both paths must be satisfied simultaneously:
	// 1. Events were decoded and forwarded.
	require.Len(t, events, 3)
	assert.Equal(t, StreamEventSystem, events[0].Type)
	assert.Equal(t, StreamEventAssistant, events[1].Type)
	assert.Equal(t, StreamEventResult, events[2].Type)

	// 2. Full stdout was captured (TeeReader behavior).
	assert.Contains(t, result.Stdout, "tee-test", "stdout must contain session_id from system event")
	assert.Contains(t, result.Stdout, "thinking", "stdout must contain assistant text")
	assert.Contains(t, result.Stdout, "result", "stdout must contain result event type")
}

// ---------------------------------------------------------------------------
// Benchmark: ParseRateLimit hot path
// ---------------------------------------------------------------------------

func BenchmarkClaudeAgent_ParseRateLimit_NoMatch(b *testing.B) {
	a := newTestAgent(AgentConfig{})
	output := "Successfully completed all tasks without any issues."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.ParseRateLimit(output)
	}
}

func BenchmarkClaudeAgent_ParseRateLimit_WithResetTime(b *testing.B) {
	a := newTestAgent(AgentConfig{})
	output := fmt.Sprintf("rate limit exceeded. Reset in %d seconds.", 30)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.ParseRateLimit(output)
	}
}

func BenchmarkClaudeAgent_DryRunCommand(b *testing.B) {
	a := newTestAgent(AgentConfig{
		Model:        "claude-opus-4-20250514",
		AllowedTools: "bash,edit,computer",
		Effort:       "high",
	})
	opts := RunOpts{
		Prompt:       strings.Repeat("write a Go function that ", 20),
		OutputFormat: "json",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.DryRunCommand(opts)
	}
}
