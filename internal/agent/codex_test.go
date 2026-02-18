package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestCodexAgent returns a CodexAgent configured with a noop logger.
func newTestCodexAgent(cfg AgentConfig) *CodexAgent {
	return NewCodexAgent(cfg, noopLogger{})
}

// ---------------------------------------------------------------------------
// NewCodexAgent / Name
// ---------------------------------------------------------------------------

func TestCodexAgent_ImplementsAgent(t *testing.T) {
	t.Parallel()
	var _ Agent = (*CodexAgent)(nil)
}

func TestCodexAgent_Name(t *testing.T) {
	t.Parallel()
	a := newTestCodexAgent(AgentConfig{})
	assert.Equal(t, "codex", a.Name())
}

func TestNewCodexAgent_NilLogger(t *testing.T) {
	t.Parallel()
	// Should not panic with nil logger.
	a := NewCodexAgent(AgentConfig{}, nil)
	assert.Equal(t, "codex", a.Name())
}

// ---------------------------------------------------------------------------
// CheckPrerequisites
// ---------------------------------------------------------------------------

func TestCodexAgent_CheckPrerequisites_FoundCommand(t *testing.T) {
	t.Parallel()
	// "sh" is guaranteed to exist on macOS/Linux.
	a := newTestCodexAgent(AgentConfig{Command: "sh"})
	assert.NoError(t, a.CheckPrerequisites())
}

func TestCodexAgent_CheckPrerequisites_MissingCommand(t *testing.T) {
	t.Parallel()
	a := newTestCodexAgent(AgentConfig{Command: "raven-nonexistent-codex-xyz"})
	err := a.CheckPrerequisites()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "codex CLI not found")
	assert.Contains(t, err.Error(), "raven-nonexistent-codex-xyz")
	assert.Contains(t, err.Error(), "https://")
}

func TestCodexAgent_CheckPrerequisites_EmptyCommandDefaultsToCodex(t *testing.T) {
	t.Parallel()
	a := newTestCodexAgent(AgentConfig{})
	err := a.CheckPrerequisites()
	if err != nil {
		// codex not installed -- that's the expected path; verify message contains "codex".
		assert.Contains(t, err.Error(), "codex")
	}
	// If codex IS installed, no error is returned. Both outcomes are valid.
}

func TestCodexAgent_CheckPrerequisites_MissingHasInstallHint(t *testing.T) {
	t.Parallel()
	a := newTestCodexAgent(AgentConfig{Command: "raven-definitely-not-installed-codex-abc"})
	err := a.CheckPrerequisites()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "https://")
}

// ---------------------------------------------------------------------------
// ParseRateLimit -- short decimal-seconds format
// ---------------------------------------------------------------------------

func TestCodexAgent_ParseRateLimit_ShortFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		output      string
		wantLimited bool
		wantAfterGT time.Duration
		wantAfterLT time.Duration
	}{
		{
			name:        "please try again in 5.448s",
			output:      "Please try again in 5.448s",
			wantLimited: true,
			wantAfterGT: 5 * time.Second,
			wantAfterLT: 6 * time.Second,
		},
		{
			name:        "try again in 2.482s lowercase",
			output:      "try again in 2.482s",
			wantLimited: true,
			wantAfterGT: 2 * time.Second,
			wantAfterLT: 3 * time.Second,
		},
		{
			name:        "try again in 0.5s",
			output:      "try again in 0.5s",
			wantLimited: true,
			wantAfterGT: 0,
			wantAfterLT: time.Second,
		},
		{
			name:        "integer seconds",
			output:      "try again in 10s",
			wantLimited: true,
			wantAfterGT: 9 * time.Second,
			wantAfterLT: 11 * time.Second,
		},
	}

	a := newTestCodexAgent(AgentConfig{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info, limited := a.ParseRateLimit(tt.output)
			require.NotNil(t, info)
			assert.True(t, limited)
			assert.True(t, info.IsLimited)
			assert.Greater(t, info.ResetAfter, tt.wantAfterGT)
			assert.Less(t, info.ResetAfter, tt.wantAfterLT)
			assert.Equal(t, tt.output, info.Message)
		})
	}
}

// ---------------------------------------------------------------------------
// ParseRateLimit -- long format
// ---------------------------------------------------------------------------

func TestCodexAgent_ParseRateLimit_LongFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		output    string
		wantAfter time.Duration
	}{
		{
			name:      "1 day",
			output:    "try again in 1 days",
			wantAfter: 24 * time.Hour,
		},
		{
			name:      "2 hours",
			output:    "try again in 2 hours",
			wantAfter: 2 * time.Hour,
		},
		{
			name:      "30 minutes",
			output:    "try again in 30 minutes",
			wantAfter: 30 * time.Minute,
		},
		{
			name:      "45 seconds",
			output:    "try again in 45 seconds",
			wantAfter: 45 * time.Second,
		},
		{
			name:      "1 day 2 hours 30 minutes 15 seconds",
			output:    "try again in 1 days 2 hours 30 minutes 15 seconds",
			wantAfter: 24*time.Hour + 2*time.Hour + 30*time.Minute + 15*time.Second,
		},
		{
			name:      "2 hours 45 minutes",
			output:    "try again in 2 hours 45 minutes",
			wantAfter: 2*time.Hour + 45*time.Minute,
		},
	}

	a := newTestCodexAgent(AgentConfig{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info, limited := a.ParseRateLimit(tt.output)
			require.NotNil(t, info, "expected rate limit info for: %s", tt.output)
			assert.True(t, limited)
			assert.True(t, info.IsLimited)
			assert.Equal(t, tt.wantAfter, info.ResetAfter)
			assert.Equal(t, tt.output, info.Message)
		})
	}
}

// ---------------------------------------------------------------------------
// ParseRateLimit -- fallback keyword
// ---------------------------------------------------------------------------

func TestCodexAgent_ParseRateLimit_FallbackKeyword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
	}{
		{name: "rate limit reached", output: "Rate limit reached"},
		{name: "rate limit lowercase", output: "rate limit reached"},
		{name: "rate limit no reached", output: "rate limit exceeded"},
		{name: "ratelimit no space", output: "ratelimit hit"},
	}

	a := newTestCodexAgent(AgentConfig{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info, limited := a.ParseRateLimit(tt.output)
			require.NotNil(t, info)
			assert.True(t, limited)
			assert.True(t, info.IsLimited)
			assert.Equal(t, time.Duration(0), info.ResetAfter)
			assert.Equal(t, tt.output, info.Message)
		})
	}
}

// ---------------------------------------------------------------------------
// ParseRateLimit -- no match
// ---------------------------------------------------------------------------

func TestCodexAgent_ParseRateLimit_NoMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
	}{
		{name: "empty string", output: ""},
		{name: "normal output", output: "Successfully ran the task."},
		{name: "error without rate limit", output: "Error: something went wrong"},
		{name: "partial rate word", output: "My rate is fine"},
	}

	a := newTestCodexAgent(AgentConfig{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info, limited := a.ParseRateLimit(tt.output)
			assert.Nil(t, info)
			assert.False(t, limited)
		})
	}
}

func TestCodexAgent_ParseRateLimit_MessagePreserved(t *testing.T) {
	t.Parallel()

	output := "Rate limit reached at 2026-01-01T00:00:00Z"
	a := newTestCodexAgent(AgentConfig{})
	info, limited := a.ParseRateLimit(output)

	require.True(t, limited)
	require.NotNil(t, info)
	assert.Equal(t, output, info.Message, "original message must be preserved verbatim")
}

// ---------------------------------------------------------------------------
// parseCodexDuration (package-level helper)
// ---------------------------------------------------------------------------

func TestParseCodexDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		match []string // [full, days, hours, minutes, seconds]
		want  time.Duration
	}{
		{
			name:  "all components",
			match: []string{"", "1", "2", "30", "15"},
			want:  24*time.Hour + 2*time.Hour + 30*time.Minute + 15*time.Second,
		},
		{
			name:  "only days",
			match: []string{"", "2", "", "", ""},
			want:  48 * time.Hour,
		},
		{
			name:  "only hours",
			match: []string{"", "", "3", "", ""},
			want:  3 * time.Hour,
		},
		{
			name:  "only minutes",
			match: []string{"", "", "", "45", ""},
			want:  45 * time.Minute,
		},
		{
			name:  "only seconds integer",
			match: []string{"", "", "", "", "30"},
			want:  30 * time.Second,
		},
		{
			name:  "fractional seconds",
			match: []string{"", "", "", "", "5.448"},
			want:  time.Duration(5.448 * float64(time.Second)),
		},
		{
			name:  "all empty components",
			match: []string{"", "", "", "", ""},
			want:  0,
		},
		{
			name:  "hours and minutes",
			match: []string{"", "", "2", "30", ""},
			want:  2*time.Hour + 30*time.Minute,
		},
		{
			name:  "match too short returns zero",
			match: []string{""},
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseCodexDuration(tt.match)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// DryRunCommand
// ---------------------------------------------------------------------------

func TestCodexAgent_DryRunCommand_BasicStructure(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{Prompt: "hello world"})

	assert.True(t, strings.HasPrefix(cmd, "codex "), "must start with 'codex '")
	assert.Contains(t, cmd, "exec")
	assert.Contains(t, cmd, "--sandbox")
	assert.Contains(t, cmd, "--ephemeral")
	assert.Contains(t, cmd, "-a")
	assert.Contains(t, cmd, "never")
	assert.Contains(t, cmd, "--prompt")
	assert.Contains(t, cmd, "hello world")
}

func TestCodexAgent_DryRunCommand_CustomCommand(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{Command: "my-codex"})
	cmd := a.DryRunCommand(RunOpts{Prompt: "test"})
	assert.True(t, strings.HasPrefix(cmd, "my-codex "))
}

func TestCodexAgent_DryRunCommand_ModelFromOpts(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{Model: "config-model"})
	cmd := a.DryRunCommand(RunOpts{Model: "opts-model", Prompt: "p"})
	assert.Contains(t, cmd, "opts-model")
	assert.NotContains(t, cmd, "config-model")
}

func TestCodexAgent_DryRunCommand_ModelFromConfig(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{Model: "gpt-4o"})
	cmd := a.DryRunCommand(RunOpts{Prompt: "p"})
	assert.Contains(t, cmd, "--model")
	assert.Contains(t, cmd, "gpt-4o")
}

func TestCodexAgent_DryRunCommand_NoModelWhenEmpty(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{Prompt: "p"})
	assert.NotContains(t, cmd, "--model")
}

func TestCodexAgent_DryRunCommand_PromptFile(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{PromptFile: "/tmp/myfile.md"})
	assert.Contains(t, cmd, "--prompt-file")
	assert.Contains(t, cmd, "/tmp/myfile.md")
	assert.NotContains(t, cmd, "--prompt ")
}

func TestCodexAgent_DryRunCommand_LargePromptTruncated(t *testing.T) {
	t.Parallel()

	// Build a prompt that exceeds maxDryRunPromptLen.
	bigPrompt := strings.Repeat("a", maxDryRunPromptLen+50)
	a := newTestCodexAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{Prompt: bigPrompt})

	// The output must contain "..." indicating truncation.
	assert.Contains(t, cmd, "...")
	// The output must NOT contain the full prompt (it was truncated).
	assert.NotContains(t, cmd, bigPrompt)
	// The truncated prompt portion must be at most maxDryRunPromptLen chars + "...".
	assert.Less(t, len(cmd), len(bigPrompt)+100) // sanity check: way shorter than full
}

func TestCodexAgent_DryRunCommand_ShortPromptNotTruncated(t *testing.T) {
	t.Parallel()

	prompt := "a short prompt"
	a := newTestCodexAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{Prompt: prompt})
	assert.Contains(t, cmd, prompt)
	assert.NotContains(t, cmd, "...")
}

func TestCodexAgent_DryRunCommand_NoPromptFlags(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{})
	assert.NotContains(t, cmd, "--prompt")
	assert.NotContains(t, cmd, "--prompt-file")
}

func TestCodexAgent_DryRunCommand_NoAllowedToolsFlag(t *testing.T) {
	t.Parallel()

	// Codex does not use --allowedTools.
	a := newTestCodexAgent(AgentConfig{AllowedTools: "bash,edit"})
	cmd := a.DryRunCommand(RunOpts{Prompt: "p"})
	assert.NotContains(t, cmd, "--allowedTools")
	assert.NotContains(t, cmd, "bash,edit")
}

func TestCodexAgent_DryRunCommand_NoPermissionModeFlag(t *testing.T) {
	t.Parallel()

	// Codex does not use --permission-mode or --print.
	a := newTestCodexAgent(AgentConfig{})
	cmd := a.DryRunCommand(RunOpts{Prompt: "p"})
	assert.NotContains(t, cmd, "--permission-mode")
	assert.NotContains(t, cmd, "--print")
}

// ---------------------------------------------------------------------------
// buildCommand
// ---------------------------------------------------------------------------

func TestCodexAgent_BuildCommand_DefaultArgs(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{})
	ctx := context.Background()
	cmd := a.buildCommand(ctx, RunOpts{})

	// Check the fixed flags appear in order.
	require.GreaterOrEqual(t, len(cmd.Args), 5)
	assert.Equal(t, "exec", cmd.Args[1])
	assert.Equal(t, "--sandbox", cmd.Args[2])
	assert.Equal(t, "--ephemeral", cmd.Args[3])
	assert.Equal(t, "-a", cmd.Args[4])
	assert.Equal(t, "never", cmd.Args[5])
}

func TestCodexAgent_BuildCommand_WorkDir(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{})
	ctx := context.Background()
	cmd := a.buildCommand(ctx, RunOpts{WorkDir: "/tmp"})
	assert.Equal(t, "/tmp", cmd.Dir)
}

func TestCodexAgent_BuildCommand_NoWorkDir(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{})
	ctx := context.Background()
	cmd := a.buildCommand(ctx, RunOpts{})
	assert.Equal(t, "", cmd.Dir)
}

func TestCodexAgent_BuildCommand_AdditionalEnv(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{})
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

func TestCodexAgent_BuildCommand_NoEffortEnvVar(t *testing.T) {
	// Cannot be parallel -- t.Setenv requires sequential test to clear
	// CLAUDE_CODE_EFFORT_LEVEL that may be set in the parent environment.
	t.Setenv("CLAUDE_CODE_EFFORT_LEVEL", "")

	// Codex does not set CLAUDE_CODE_EFFORT_LEVEL regardless of Effort config.
	a := newTestCodexAgent(AgentConfig{Effort: "high"})
	ctx := context.Background()
	cmd := a.buildCommand(ctx, RunOpts{Effort: "high"})

	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "CLAUDE_CODE_EFFORT_LEVEL=") {
			assert.Equal(t, "CLAUDE_CODE_EFFORT_LEVEL=", e,
				"codex must not set a non-empty CLAUDE_CODE_EFFORT_LEVEL")
		}
	}
}

// ---------------------------------------------------------------------------
// Run (unit-level)
// ---------------------------------------------------------------------------

func TestCodexAgent_Run_SuccessWithEcho(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{Command: "echo"})
	ctx := context.Background()

	result, err := a.Run(ctx, RunOpts{})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.True(t, result.Success())
	assert.Greater(t, result.Duration, time.Duration(0))
}

func TestCodexAgent_Run_NonZeroExitCode(t *testing.T) {
	t.Parallel()

	// "false" exits with code 1 on all POSIX systems.
	a := newTestCodexAgent(AgentConfig{Command: "false"})
	ctx := context.Background()

	result, err := a.Run(ctx, RunOpts{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	assert.False(t, result.Success())
}

func TestCodexAgent_Run_CommandNotFound(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{Command: "this-binary-does-not-exist-codex-xyz"})
	_, err := a.Run(context.Background(), RunOpts{})
	require.Error(t, err, "Run must return an error when the command binary is missing")
	assert.Contains(t, err.Error(), "starting codex")
}

func TestCodexAgent_Run_RateLimitNotDetectedForNormalOutput(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{Command: "echo"})
	result, err := a.Run(context.Background(), RunOpts{})
	require.NoError(t, err)
	assert.False(t, result.WasRateLimited())
	assert.Nil(t, result.RateLimit)
}

func TestCodexAgent_Run_ContextCancellation(t *testing.T) {
	t.Parallel()

	a := newTestCodexAgent(AgentConfig{Command: "sh"})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := a.Run(ctx, RunOpts{})
	// Either the context kills the process or sh exits immediately. Both are fine.
	if err != nil {
		t.Logf("Run returned error (acceptable): %v", err)
	}
}

// ---------------------------------------------------------------------------
// Run integration tests using mock shell scripts
// ---------------------------------------------------------------------------

func TestCodexAgent_Run_Integration_StdoutAndStderrCaptured(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "codex-success.sh", `
echo "Task completed"
echo "Debug info" >&2
exit 0
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.True(t, result.Success())
	assert.Contains(t, result.Stdout, "Task completed")
	assert.Contains(t, result.Stderr, "Debug info")
	assert.Greater(t, result.Duration, time.Duration(0))
	assert.Nil(t, result.RateLimit)
}

func TestCodexAgent_Run_Integration_NonZeroExitCode(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "codex-nonzero.sh", `
echo "partial output"
exit 2
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err, "Run should not return a Go error for non-zero exit codes")
	assert.Equal(t, 2, result.ExitCode)
	assert.False(t, result.Success())
	assert.Contains(t, result.Stdout, "partial output")
}

func TestCodexAgent_Run_Integration_ShortFormatRateLimitDetected(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "codex-rate-short.sh", `
echo "Please try again in 5.448s"
exit 1
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	require.NotNil(t, result.RateLimit)
	assert.True(t, result.WasRateLimited())
	assert.Greater(t, result.RateLimit.ResetAfter, 5*time.Second)
	assert.Less(t, result.RateLimit.ResetAfter, 6*time.Second)
}

func TestCodexAgent_Run_Integration_LongFormatRateLimitDetected(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "codex-rate-long.sh", `
echo "try again in 2 hours 30 minutes"
exit 1
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	require.NotNil(t, result.RateLimit)
	assert.True(t, result.RateLimit.IsLimited)
	assert.Equal(t, 2*time.Hour+30*time.Minute, result.RateLimit.ResetAfter)
}

func TestCodexAgent_Run_Integration_FallbackRateLimitDetected(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "codex-rate-fallback.sh", `
echo "Rate limit reached for your account"
exit 1
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	require.NotNil(t, result.RateLimit)
	assert.True(t, result.RateLimit.IsLimited)
	assert.Equal(t, time.Duration(0), result.RateLimit.ResetAfter)
}

func TestCodexAgent_Run_Integration_ContextCancellationKillsProcess(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "codex-slow.sh", `
sleep 60
echo "should not reach here"
exit 0
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := a.Run(ctx, RunOpts{})
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 5*time.Second, "subprocess should have been killed promptly on context cancellation")
	if err != nil {
		t.Logf("Run returned error after context cancellation (acceptable): %v", err)
	}
}

func TestCodexAgent_Run_Integration_WorkDirUsed(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	workDir := t.TempDir()
	scriptDir := t.TempDir()

	scriptPath := writeMockScript(t, scriptDir, "codex-pwd.sh", `
pwd
exit 0
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{WorkDir: workDir})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, filepath.Base(workDir))
}

func TestCodexAgent_Run_Integration_ExtraEnvMerged(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "codex-env.sh", `
echo "RAVEN_TEST_VAR=$RAVEN_TEST_VAR"
exit 0
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{
		Env: []string{"RAVEN_TEST_VAR=codex_integration_test_value"},
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "RAVEN_TEST_VAR=codex_integration_test_value")
}

func TestCodexAgent_Run_Integration_ModelFlagSet(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "codex-model.sh", `
echo "args: $*"
exit 0
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath, Model: "gpt-4o"})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "--model")
	assert.Contains(t, result.Stdout, "gpt-4o")
}

func TestCodexAgent_Run_Integration_PromptPassedAsFlag(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "codex-args.sh", `
echo "args: $*"
exit 0
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{Prompt: "implement the feature"})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "--prompt")
	assert.Contains(t, result.Stdout, "implement the feature")
}

func TestCodexAgent_Run_Integration_PromptFileFlag(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "my-prompt.md")
	err := os.WriteFile(promptFile, []byte("# My Prompt\nDo the thing."), 0644)
	require.NoError(t, err)

	scriptDir := t.TempDir()
	scriptPath := writeMockScript(t, scriptDir, "codex-prompt-file.sh", `
echo "args: $*"
exit 0
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{PromptFile: promptFile})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "--prompt-file")
	assert.Contains(t, result.Stdout, promptFile)
}

func TestCodexAgent_Run_Integration_FixedFlagsAlwaysPresent(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "codex-flags.sh", `
echo "args: $*"
exit 0
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "exec")
	assert.Contains(t, result.Stdout, "--sandbox")
	assert.Contains(t, result.Stdout, "--ephemeral")
	assert.Contains(t, result.Stdout, "-a")
	assert.Contains(t, result.Stdout, "never")
}

func TestCodexAgent_Run_Integration_DurationMeasured(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	dir := t.TempDir()
	scriptPath := writeMockScript(t, dir, "codex-duration.sh", `
echo "done"
exit 0
`)

	a := newTestCodexAgent(AgentConfig{Command: scriptPath})
	result, err := a.Run(context.Background(), RunOpts{})

	require.NoError(t, err)
	assert.Greater(t, result.Duration, time.Duration(0), "Duration must be positive")
}

// ---------------------------------------------------------------------------
// CheckPrerequisites integration
// ---------------------------------------------------------------------------

func TestCodexAgent_CheckPrerequisites_CustomCommandOnPath(t *testing.T) {
	// NOTE: t.Setenv modifies os-level PATH so this test must NOT be parallel.
	skipOnWindows(t)

	dir := t.TempDir()
	writeMockScript(t, dir, "fake-codex", `exit 0`)

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	a := newTestCodexAgent(AgentConfig{Command: "fake-codex"})
	err := a.CheckPrerequisites()
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Name variations
// ---------------------------------------------------------------------------

func TestCodexAgent_Name_ReturnsCodexString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config AgentConfig
	}{
		{name: "empty config", config: AgentConfig{}},
		{name: "with model", config: AgentConfig{Model: "gpt-4o"}},
		{name: "with command", config: AgentConfig{Command: "my-codex"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := newTestCodexAgent(tt.config)
			assert.Equal(t, "codex", a.Name())
		})
	}
}

// ---------------------------------------------------------------------------
// Benchmark: ParseRateLimit hot path
// ---------------------------------------------------------------------------

func BenchmarkCodexAgent_ParseRateLimit_NoMatch(b *testing.B) {
	a := newTestCodexAgent(AgentConfig{})
	output := "Successfully completed all tasks without any issues."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.ParseRateLimit(output)
	}
}

func BenchmarkCodexAgent_ParseRateLimit_ShortFormat(b *testing.B) {
	a := newTestCodexAgent(AgentConfig{})
	output := "Please try again in 5.448s"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.ParseRateLimit(output)
	}
}

func BenchmarkCodexAgent_ParseRateLimit_LongFormat(b *testing.B) {
	a := newTestCodexAgent(AgentConfig{})
	output := "try again in 1 days 2 hours 30 minutes 15 seconds"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.ParseRateLimit(output)
	}
}

func BenchmarkCodexAgent_DryRunCommand(b *testing.B) {
	a := newTestCodexAgent(AgentConfig{
		Model: "gpt-4o",
	})
	opts := RunOpts{
		Prompt: strings.Repeat("write a Go function that ", 20),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.DryRunCommand(opts)
	}
}

// ---------------------------------------------------------------------------
// Verify Codex does not use Claude-specific flags
// ---------------------------------------------------------------------------

func TestCodexAgent_DryRunCommand_NoClaudeSpecificFlags(t *testing.T) {
	t.Parallel()

	// Ensure the codex adapter does not accidentally use Claude-only flags.
	a := newTestCodexAgent(AgentConfig{
		Model:        "gpt-4o",
		AllowedTools: "bash,edit",
		Effort:       "high",
	})
	cmd := a.DryRunCommand(RunOpts{
		Prompt:       "do the thing",
		OutputFormat: "json",
		Effort:       "high",
		AllowedTools: "bash",
	})

	assert.NotContains(t, cmd, "--permission-mode")
	assert.NotContains(t, cmd, "--print")
	assert.NotContains(t, cmd, "--allowedTools")
	assert.NotContains(t, cmd, "--output-format")
}

// skipOnWindows and writeMockScript are defined in claude_test.go; both files
// are in the same package so they are visible here automatically.
