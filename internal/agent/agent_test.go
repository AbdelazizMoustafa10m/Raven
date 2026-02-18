package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Registry tests
// ---------------------------------------------------------------------------

func TestNewRegistry(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	require.NotNil(t, r)
	assert.Empty(t, r.List())
}

func TestRegistry_Register_Get_RoundTrip(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	mock := NewMockAgent("claude")

	err := r.Register(mock)
	require.NoError(t, err)

	got, err := r.Get("claude")
	require.NoError(t, err)
	assert.Equal(t, mock, got)
}

func TestRegistry_Register_DuplicateName(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	err := r.Register(NewMockAgent("claude"))
	require.NoError(t, err)

	err = r.Register(NewMockAgent("claude"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateName)
}

func TestRegistry_Register_NilAgent(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	err := r.Register(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidName)
}

func TestRegistry_Register_InvalidNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		agentName string
	}{
		{name: "empty name", agentName: ""},
		{name: "starts with hyphen", agentName: "-claude"},
		{name: "contains space", agentName: "my agent"},
		{name: "contains slash", agentName: "claude/3"},
		{name: "contains underscore", agentName: "my_agent"},
		{name: "contains dot", agentName: "claude.ai"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := NewRegistry()
			err := r.Register(NewMockAgent(tt.agentName))
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidName)
		})
	}
}

func TestRegistry_Register_ValidNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		agentName string
	}{
		{name: "simple lowercase", agentName: "claude"},
		{name: "with hyphen", agentName: "claude-3"},
		{name: "with numbers", agentName: "gpt4"},
		{name: "uppercase letters", agentName: "Gemini"},
		{name: "mixed", agentName: "codex-v2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := NewRegistry()
			err := r.Register(NewMockAgent(tt.agentName))
			assert.NoError(t, err)
		})
	}
}

func TestRegistry_Get_UnknownName(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	_, err := r.Get("nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRegistry_Get_EmptyRegistry(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	_, err := r.Get("anything")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRegistry_MustGet_Success(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	mock := NewMockAgent("claude")
	require.NoError(t, r.Register(mock))

	got := r.MustGet("claude")
	assert.Equal(t, mock, got)
}

func TestRegistry_MustGet_Panics(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	assert.Panics(t, func() {
		r.MustGet("nonexistent")
	})
}

func TestRegistry_List_SortedAlphabetically(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	require.NoError(t, r.Register(NewMockAgent("gemini")))
	require.NoError(t, r.Register(NewMockAgent("claude")))
	require.NoError(t, r.Register(NewMockAgent("codex")))

	list := r.List()
	assert.Equal(t, []string{"claude", "codex", "gemini"}, list)
}

func TestRegistry_List_EmptyRegistry(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	list := r.List()
	assert.NotNil(t, list)
	assert.Empty(t, list)
}

func TestRegistry_Has_ReturnsTrueForRegistered(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	require.NoError(t, r.Register(NewMockAgent("claude")))

	assert.True(t, r.Has("claude"))
}

func TestRegistry_Has_ReturnsFalseForUnregistered(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	assert.False(t, r.Has("claude"))
	assert.False(t, r.Has(""))
}

func TestRegistry_ConcurrentReads(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	for i := 0; i < 10; i++ {
		require.NoError(t, r.Register(NewMockAgent(fmt.Sprintf("agent-%d", i))))
	}

	const workers = 50
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			_, _ = r.Get(fmt.Sprintf("agent-%d", i%10))
			_ = r.Has(fmt.Sprintf("agent-%d", i%10))
			_ = r.List()
			errs <- nil
		}(i)
	}
	for i := 0; i < workers; i++ {
		<-errs
	}
}

// Integration test: register 3 agents, iterate, verify each produces output.
func TestRegistry_Integration_MultipleAgents(t *testing.T) {
	t.Parallel()

	r := NewRegistry()

	agents := []*MockAgent{
		NewMockAgent("claude"),
		NewMockAgent("codex"),
		NewMockAgent("gemini"),
	}
	for _, a := range agents {
		require.NoError(t, r.Register(a))
	}

	names := r.List()
	assert.Equal(t, []string{"claude", "codex", "gemini"}, names)

	ctx := context.Background()
	opts := RunOpts{Prompt: "hello"}

	for _, name := range names {
		a, err := r.Get(name)
		require.NoError(t, err)

		result, err := a.Run(ctx, opts)
		require.NoError(t, err)
		assert.True(t, result.Success())
		assert.Equal(t, "mock output", result.Stdout)
	}
}

// ---------------------------------------------------------------------------
// MockAgent tests
// ---------------------------------------------------------------------------

func TestMockAgent_ImplementsAgent(t *testing.T) {
	t.Parallel()

	var _ Agent = (*MockAgent)(nil)
}

func TestMockAgent_Name(t *testing.T) {
	t.Parallel()

	m := NewMockAgent("claude")
	assert.Equal(t, "claude", m.Name())
}

func TestMockAgent_Run_DefaultBehavior(t *testing.T) {
	t.Parallel()

	m := NewMockAgent("claude")
	result, err := m.Run(context.Background(), RunOpts{Prompt: "test"})
	require.NoError(t, err)
	assert.Equal(t, "mock output", result.Stdout)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, 100*time.Millisecond, result.Duration)
	assert.True(t, result.Success())
}

func TestMockAgent_Run_RecordsCalls(t *testing.T) {
	t.Parallel()

	m := NewMockAgent("claude")
	opts1 := RunOpts{Prompt: "first call"}
	opts2 := RunOpts{Prompt: "second call", Model: "gpt-4"}

	_, err := m.Run(context.Background(), opts1)
	require.NoError(t, err)
	_, err = m.Run(context.Background(), opts2)
	require.NoError(t, err)

	require.Len(t, m.Calls, 2)
	assert.Equal(t, opts1, m.Calls[0])
	assert.Equal(t, opts2, m.Calls[1])
}

func TestMockAgent_Run_CustomRunFunc(t *testing.T) {
	t.Parallel()

	customErr := errors.New("custom error")
	m := NewMockAgent("claude").WithRunFunc(func(ctx context.Context, opts RunOpts) (*RunResult, error) {
		return nil, customErr
	})

	result, err := m.Run(context.Background(), RunOpts{Prompt: "test"})
	require.Error(t, err)
	assert.ErrorIs(t, err, customErr)
	assert.Nil(t, result)
	// Call was still recorded.
	assert.Len(t, m.Calls, 1)
}

func TestMockAgent_CheckPrerequisites_Default(t *testing.T) {
	t.Parallel()

	m := NewMockAgent("claude")
	assert.NoError(t, m.CheckPrerequisites())
}

func TestMockAgent_CheckPrerequisites_WithError(t *testing.T) {
	t.Parallel()

	prereqErr := errors.New("claude CLI not found")
	m := NewMockAgent("claude").WithPrereqError(prereqErr)

	err := m.CheckPrerequisites()
	require.Error(t, err)
	assert.ErrorIs(t, err, prereqErr)
}

func TestMockAgent_ParseRateLimit_Default(t *testing.T) {
	t.Parallel()

	m := NewMockAgent("claude")
	info, limited := m.ParseRateLimit("some output")
	assert.Nil(t, info)
	assert.False(t, limited)
}

func TestMockAgent_ParseRateLimit_WithRateLimit(t *testing.T) {
	t.Parallel()

	m := NewMockAgent("claude").WithRateLimit(30 * time.Second)

	info, limited := m.ParseRateLimit("rate limit exceeded")
	require.NotNil(t, info)
	assert.True(t, limited)
	assert.True(t, info.IsLimited)
	assert.Equal(t, 30*time.Second, info.ResetAfter)
	assert.Equal(t, "mock rate limit", info.Message)
}

func TestMockAgent_DryRunCommand_Default(t *testing.T) {
	t.Parallel()

	m := NewMockAgent("claude")
	cmd := m.DryRunCommand(RunOpts{Prompt: "hello world"})
	assert.Equal(t, `mock-agent --prompt "hello world"`, cmd)
}

func TestMockAgent_DryRunCommand_Custom(t *testing.T) {
	t.Parallel()

	m := NewMockAgent("claude")
	m.DryRunOutput = "custom-command --flag"
	cmd := m.DryRunCommand(RunOpts{Prompt: "ignored"})
	assert.Equal(t, "custom-command --flag", cmd)
}

func TestMockAgent_BuilderMethodChaining(t *testing.T) {
	t.Parallel()

	customErr := errors.New("no tool")
	customResult := &RunResult{Stdout: "custom", ExitCode: 0, Duration: time.Second}

	m := NewMockAgent("test").
		WithRunFunc(func(ctx context.Context, opts RunOpts) (*RunResult, error) {
			return customResult, nil
		}).
		WithRateLimit(60 * time.Second).
		WithPrereqError(customErr)

	// Verify RunFunc.
	result, err := m.Run(context.Background(), RunOpts{})
	require.NoError(t, err)
	assert.Equal(t, customResult, result)

	// Verify rate limit.
	info, limited := m.ParseRateLimit("")
	assert.True(t, limited)
	assert.Equal(t, 60*time.Second, info.ResetAfter)

	// Verify prereq error.
	assert.ErrorIs(t, m.CheckPrerequisites(), customErr)
}

// ---------------------------------------------------------------------------
// AgentConfig tests
// ---------------------------------------------------------------------------

func TestAgentConfig_Fields(t *testing.T) {
	t.Parallel()

	cfg := AgentConfig{
		Command:        "claude",
		Model:          "claude-sonnet-4-20250514",
		Effort:         "high",
		PromptTemplate: "implement",
		AllowedTools:   "bash,edit",
	}

	assert.Equal(t, "claude", cfg.Command)
	assert.Equal(t, "claude-sonnet-4-20250514", cfg.Model)
	assert.Equal(t, "high", cfg.Effort)
	assert.Equal(t, "implement", cfg.PromptTemplate)
	assert.Equal(t, "bash,edit", cfg.AllowedTools)
}

// ---------------------------------------------------------------------------
// Sentinel error tests
// ---------------------------------------------------------------------------

func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, ErrNotFound)
	assert.NotNil(t, ErrDuplicateName)
	assert.NotNil(t, ErrInvalidName)

	// Ensure they are distinct.
	assert.NotEqual(t, ErrNotFound, ErrDuplicateName)
	assert.NotEqual(t, ErrNotFound, ErrInvalidName)
	assert.NotEqual(t, ErrDuplicateName, ErrInvalidName)
}

func TestRegistry_ErrorMessages_AreDescriptive(t *testing.T) {
	t.Parallel()

	r := NewRegistry()

	_, err := r.Get("ghost")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")

	err = r.Register(NewMockAgent("dup"))
	require.NoError(t, err)
	err = r.Register(NewMockAgent("dup"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dup")
}
