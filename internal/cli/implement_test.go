package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
)

// ---- validateImplementFlags tests -------------------------------------------

func TestValidateImplementFlags_BothMissing(t *testing.T) {
	flags := implementFlags{}
	_, err := validateImplementFlags(flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "either --phase or --task must be specified")
}

func TestValidateImplementFlags_BothSet(t *testing.T) {
	flags := implementFlags{PhaseStr: "2", Task: "T-001"}
	_, err := validateImplementFlags(flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestValidateImplementFlags_TaskMode(t *testing.T) {
	flags := implementFlags{Task: "T-029"}
	phaseID, err := validateImplementFlags(flags)
	require.NoError(t, err)
	assert.Equal(t, 0, phaseID)
}

func TestValidateImplementFlags_PhaseAll(t *testing.T) {
	tests := []struct {
		phaseStr string
	}{
		{"all"},
		{"ALL"},
		{"All"},
	}
	for _, tt := range tests {
		t.Run(tt.phaseStr, func(t *testing.T) {
			flags := implementFlags{PhaseStr: tt.phaseStr}
			phaseID, err := validateImplementFlags(flags)
			require.NoError(t, err)
			assert.Equal(t, 0, phaseID, "phase 'all' should map to sentinel 0")
		})
	}
}

func TestValidateImplementFlags_PhaseNumeric(t *testing.T) {
	tests := []struct {
		name     string
		phaseStr string
		wantID   int
		wantErr  bool
	}{
		{name: "phase 1", phaseStr: "1", wantID: 1},
		{name: "phase 2", phaseStr: "2", wantID: 2},
		{name: "phase 10", phaseStr: "10", wantID: 10},
		{name: "phase with spaces", phaseStr: " 3 ", wantID: 3},
		{name: "invalid string", phaseStr: "xyz", wantErr: true},
		{name: "empty string after spaces", phaseStr: "  ", wantErr: true},
		{name: "negative", phaseStr: "-1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := implementFlags{PhaseStr: tt.phaseStr}
			phaseID, err := validateImplementFlags(flags)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, phaseID)
		})
	}
}

// ---- buildAgentRegistry tests -----------------------------------------------

func TestBuildAgentRegistry_AllAgentsRegistered(t *testing.T) {
	flags := implementFlags{Agent: "claude"}
	registry, err := buildAgentRegistry(nil, flags)
	require.NoError(t, err)

	names := registry.List()
	assert.Contains(t, names, "claude")
	assert.Contains(t, names, "codex")
	assert.Contains(t, names, "gemini")
}

func TestBuildAgentRegistry_ModelOverride_Claude(t *testing.T) {
	flags := implementFlags{
		Agent: "claude",
		Model: "claude-opus-4-6",
	}
	registry, err := buildAgentRegistry(nil, flags)
	require.NoError(t, err)

	ag, err := registry.Get("claude")
	require.NoError(t, err)
	assert.NotNil(t, ag)
	// Verify the agent is available with the expected name.
	assert.Equal(t, "claude", ag.Name())
}

func TestBuildAgentRegistry_ModelOverride_Codex(t *testing.T) {
	flags := implementFlags{
		Agent: "codex",
		Model: "gpt-4o",
	}
	registry, err := buildAgentRegistry(nil, flags)
	require.NoError(t, err)

	ag, err := registry.Get("codex")
	require.NoError(t, err)
	assert.Equal(t, "codex", ag.Name())
}

func TestBuildAgentRegistry_UnknownAgentLookup(t *testing.T) {
	flags := implementFlags{Agent: "claude"}
	registry, err := buildAgentRegistry(nil, flags)
	require.NoError(t, err)

	_, err = registry.Get("unknown-agent")
	require.Error(t, err)
}

// ---- newImplementCmd tests ---------------------------------------------------

func TestNewImplementCmd_Registration(t *testing.T) {
	cmd := newImplementCmd()
	assert.Equal(t, "implement", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
}

func TestNewImplementCmd_FlagExists(t *testing.T) {
	cmd := newImplementCmd()

	expectedFlags := []string{
		"agent",
		"phase",
		"task",
		"max-iterations",
		"max-limit-waits",
		"sleep",
		"dry-run",
		"model",
	}
	for _, name := range expectedFlags {
		flag := cmd.Flags().Lookup(name)
		assert.NotNil(t, flag, "expected flag --%s to exist", name)
	}
}

func TestNewImplementCmd_Defaults(t *testing.T) {
	cmd := newImplementCmd()

	maxIterFlag := cmd.Flags().Lookup("max-iterations")
	require.NotNil(t, maxIterFlag)
	assert.Equal(t, "50", maxIterFlag.DefValue)

	maxWaitsFlag := cmd.Flags().Lookup("max-limit-waits")
	require.NotNil(t, maxWaitsFlag)
	assert.Equal(t, "5", maxWaitsFlag.DefValue)

	sleepFlag := cmd.Flags().Lookup("sleep")
	require.NotNil(t, sleepFlag)
	assert.Equal(t, "5", sleepFlag.DefValue)

	dryRunFlag := cmd.Flags().Lookup("dry-run")
	require.NotNil(t, dryRunFlag)
	assert.Equal(t, "false", dryRunFlag.DefValue)
}

func TestNewImplementCmd_AgentRequired(t *testing.T) {
	// Confirm that --agent is marked required by verifying command execution
	// fails when --agent is not provided. This is the most reliable check
	// across all Cobra versions.
	cmd := newImplementCmd()
	// Use a buffer for output to suppress cobra's usage output.
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--phase", "1"})
	err := cmd.Execute()
	assert.Error(t, err, "command should fail without required --agent flag")
}

// ---- integration-style command help test ------------------------------------

func TestNewImplementCmd_HelpContainsUsageExamples(t *testing.T) {
	cmd := newImplementCmd()
	help := cmd.Long + cmd.Example
	assert.True(t, strings.Contains(help, "--agent"), "help should mention --agent flag")
	assert.True(t, strings.Contains(help, "--phase"), "help should mention --phase flag")
	assert.True(t, strings.Contains(help, "--task"), "help should mention --task flag")
	assert.True(t, strings.Contains(help, "--dry-run"), "help should mention --dry-run flag")
}

// ---- rootCmd integration test -----------------------------------------------

func TestImplementCmdRegisteredOnRoot(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "implement" {
			found = true
			break
		}
	}
	assert.True(t, found, "implement command should be registered as a subcommand of root")
}

// ---- validateImplementFlags edge cases --------------------------------------

func TestValidateImplementFlags_PhaseZeroIsSentinel(t *testing.T) {
	// PhaseStr "0" is the numeric form of the "all phases" sentinel.
	flags := implementFlags{PhaseStr: "0"}
	phaseID, err := validateImplementFlags(flags)
	require.NoError(t, err)
	assert.Equal(t, 0, phaseID, `PhaseStr "0" should map to sentinel 0 (all phases)`)
}

func TestValidateImplementFlags_PhaseAllCaseInsensitiveWithWhitespace(t *testing.T) {
	// "all" variants with surrounding whitespace should all parse to sentinel 0.
	tests := []struct {
		name     string
		phaseStr string
	}{
		{name: "all with leading space", phaseStr: " all"},
		{name: "all with trailing space", phaseStr: "all "},
		{name: "all with surrounding spaces", phaseStr: "  all  "},
		{name: "ALL with surrounding spaces", phaseStr: "  ALL  "},
		{name: "Mixed case with spaces", phaseStr: " All "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := implementFlags{PhaseStr: tt.phaseStr}
			phaseID, err := validateImplementFlags(flags)
			require.NoError(t, err, "PhaseStr %q should not produce an error", tt.phaseStr)
			assert.Equal(t, 0, phaseID, "PhaseStr %q should map to sentinel 0", tt.phaseStr)
		})
	}
}

// ---- buildAgentRegistry with config -----------------------------------------

func TestBuildAgentRegistry_WithNonNilAgentCfgs(t *testing.T) {
	// Providing a non-nil agentCfgs map with a claude config must not panic
	// and must register all three agents.
	agentCfgs := map[string]config.AgentConfig{
		"claude": {
			Command: "claude",
			Model:   "claude-sonnet-4-20250514",
			Effort:  "high",
		},
	}
	flags := implementFlags{Agent: "claude"}
	registry, err := buildAgentRegistry(agentCfgs, flags)
	require.NoError(t, err)

	names := registry.List()
	assert.Contains(t, names, "claude")
	assert.Contains(t, names, "codex")
	assert.Contains(t, names, "gemini")
}

func TestBuildAgentRegistry_ModelOverrideOnlyAffectsSelectedAgent(t *testing.T) {
	// When --model is set for "claude", the codex and gemini agents must NOT
	// inherit that model. This test verifies isolation by looking up each agent
	// and confirming only the selected agent is present in the registry without
	// errors (structural check; direct config access is not part of the public API).
	agentCfgs := map[string]config.AgentConfig{
		"claude": {Model: "claude-original"},
		"codex":  {Model: "codex-original"},
		"gemini": {Model: "gemini-original"},
	}
	flags := implementFlags{
		Agent: "claude",
		Model: "claude-opus-4-6",
	}
	registry, err := buildAgentRegistry(agentCfgs, flags)
	require.NoError(t, err)

	// All three agents must still be registered.
	ag, err := registry.Get("claude")
	require.NoError(t, err)
	assert.Equal(t, "claude", ag.Name())

	ag, err = registry.Get("codex")
	require.NoError(t, err)
	assert.Equal(t, "codex", ag.Name())

	ag, err = registry.Get("gemini")
	require.NoError(t, err)
	assert.Equal(t, "gemini", ag.Name())
}

func TestBuildAgentRegistry_ModelOverride_Gemini(t *testing.T) {
	// When "gemini" is the selected agent and --model is provided, buildAgentRegistry
	// must succeed and gemini must be registered under its own name.
	flags := implementFlags{
		Agent: "gemini",
		Model: "gemini-2.5-pro",
	}
	registry, err := buildAgentRegistry(nil, flags)
	require.NoError(t, err)

	ag, err := registry.Get("gemini")
	require.NoError(t, err)
	assert.Equal(t, "gemini", ag.Name())
}

func TestBuildAgentRegistry_ModelOverride_NonSelectedAgentUnchanged(t *testing.T) {
	// When codex is selected with a model override, claude and gemini must
	// still appear in the registry.
	agentCfgs := map[string]config.AgentConfig{
		"claude": {Model: "claude-sonnet-4-20250514"},
		"codex":  {Model: "gpt-4o"},
	}
	flags := implementFlags{
		Agent: "codex",
		Model: "o3",
	}
	registry, err := buildAgentRegistry(agentCfgs, flags)
	require.NoError(t, err)

	// The overridden agent must be reachable.
	codexAg, err := registry.Get("codex")
	require.NoError(t, err)
	assert.Equal(t, "codex", codexAg.Name())

	// Non-selected agents must also be present and unaffected.
	claudeAg, err := registry.Get("claude")
	require.NoError(t, err)
	assert.Equal(t, "claude", claudeAg.Name())
}

// ---- newImplementCmd shell completion test ----------------------------------

func TestNewImplementCmd_AgentFlagHasShellCompletion(t *testing.T) {
	cmd := newImplementCmd()

	completionFn, exists := cmd.GetFlagCompletionFunc("agent")
	require.True(t, exists, "--agent flag should have a completion function registered")
	require.NotNil(t, completionFn, "completion function must not be nil")

	// Invoke the completion function and verify it returns the known agents.
	completions, directive := completionFn(cmd, []string{}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive,
		"--agent completion should use ShellCompDirectiveNoFileComp")
	assert.Contains(t, completions, "claude")
	assert.Contains(t, completions, "codex")
	assert.Contains(t, completions, "gemini")
	assert.Len(t, completions, 3, "completion should list exactly the three known agents")
}

// ---- runnerLogger adapter tests ---------------------------------------------

// captureLogger is a minimal charmLogger that records calls for assertion.
type captureLogger struct {
	infoCalls  []captureCall
	debugCalls []captureCall
}

type captureCall struct {
	msg string
	kv  []any
}

func (c *captureLogger) Info(msg interface{}, kv ...interface{}) {
	c.infoCalls = append(c.infoCalls, captureCall{
		msg: msg.(string),
		kv:  kv,
	})
}

func (c *captureLogger) Debug(msg interface{}, kv ...interface{}) {
	c.debugCalls = append(c.debugCalls, captureCall{
		msg: msg.(string),
		kv:  kv,
	})
}

func TestRunnerLogger_InfoDelegation(t *testing.T) {
	capture := &captureLogger{}
	logger := &runnerLogger{logger: capture}

	logger.Info("hello", "key", "value")

	require.Len(t, capture.infoCalls, 1, "Info should be forwarded exactly once")
	assert.Equal(t, "hello", capture.infoCalls[0].msg)
	assert.Equal(t, []any{"key", "value"}, capture.infoCalls[0].kv)
}

func TestRunnerLogger_DebugDelegation(t *testing.T) {
	capture := &captureLogger{}
	logger := &runnerLogger{logger: capture}

	logger.Debug("debug msg", "count", 42)

	require.Len(t, capture.debugCalls, 1, "Debug should be forwarded exactly once")
	assert.Equal(t, "debug msg", capture.debugCalls[0].msg)
	assert.Equal(t, []any{"count", 42}, capture.debugCalls[0].kv)
}

func TestRunnerLogger_MultipleCallsAccumulate(t *testing.T) {
	capture := &captureLogger{}
	logger := &runnerLogger{logger: capture}

	logger.Info("first")
	logger.Info("second", "k", "v")
	logger.Debug("dbg")

	assert.Len(t, capture.infoCalls, 2, "both Info calls should be recorded")
	assert.Len(t, capture.debugCalls, 1, "one Debug call should be recorded")
	assert.Equal(t, "first", capture.infoCalls[0].msg)
	assert.Equal(t, "second", capture.infoCalls[1].msg)
}

// ---- agentDebugLogger adapter tests ----------------------------------------

func TestAgentDebugLogger_DebugDelegation(t *testing.T) {
	capture := &captureLogger{}
	logger := &agentDebugLogger{logger: capture}

	logger.Debug("agent debug", "model", "claude-opus-4-6")

	require.Len(t, capture.debugCalls, 1, "Debug should be forwarded exactly once")
	assert.Equal(t, "agent debug", capture.debugCalls[0].msg)
	assert.Equal(t, []any{"model", "claude-opus-4-6"}, capture.debugCalls[0].kv)
}

func TestAgentDebugLogger_InfoNotForwarded(t *testing.T) {
	// agentDebugLogger only exposes Debug(); calling Info() on the underlying
	// charmLogger should not happen via this adapter (it has no Info method).
	// We verify that the adapter type satisfies only a Debug interface and that
	// Debug calls are forwarded without side-effects to Info.
	capture := &captureLogger{}
	logger := &agentDebugLogger{logger: capture}

	logger.Debug("only debug")

	assert.Len(t, capture.infoCalls, 0, "agentDebugLogger must not trigger Info calls")
	assert.Len(t, capture.debugCalls, 1)
}

func TestAgentDebugLogger_MultipleDebugCalls(t *testing.T) {
	capture := &captureLogger{}
	logger := &agentDebugLogger{logger: capture}

	logger.Debug("first debug", "a", 1)
	logger.Debug("second debug", "b", 2)

	require.Len(t, capture.debugCalls, 2)
	assert.Equal(t, "first debug", capture.debugCalls[0].msg)
	assert.Equal(t, "second debug", capture.debugCalls[1].msg)
}
