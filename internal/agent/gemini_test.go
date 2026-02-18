package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// GeminiAgent tests
// ---------------------------------------------------------------------------

func TestGeminiAgent_ImplementsAgent(t *testing.T) {
	t.Parallel()

	var _ Agent = (*GeminiAgent)(nil)
}

func TestNewGeminiAgent(t *testing.T) {
	t.Parallel()

	cfg := AgentConfig{Command: "gemini", Model: "gemini-2.0-flash"}
	g := NewGeminiAgent(cfg)

	require.NotNil(t, g)
	assert.Equal(t, cfg, g.config)
}

func TestGeminiAgent_Name(t *testing.T) {
	t.Parallel()

	g := NewGeminiAgent(AgentConfig{})
	assert.Equal(t, "gemini", g.Name())
}

func TestGeminiAgent_Run_ReturnsErrNotImplemented(t *testing.T) {
	t.Parallel()

	g := NewGeminiAgent(AgentConfig{})
	result, err := g.Run(context.Background(), RunOpts{Prompt: "hello"})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotImplemented)
	assert.Nil(t, result)
}

func TestGeminiAgent_Run_ContextCancelledStillReturnsErrNotImplemented(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	g := NewGeminiAgent(AgentConfig{})
	result, err := g.Run(ctx, RunOpts{Prompt: "hello"})

	require.Error(t, err)
	// The stub ignores context; ErrNotImplemented is always returned.
	assert.ErrorIs(t, err, ErrNotImplemented)
	assert.Nil(t, result)
}

func TestGeminiAgent_CheckPrerequisites_ReturnsErrNotImplemented(t *testing.T) {
	t.Parallel()

	g := NewGeminiAgent(AgentConfig{})
	err := g.CheckPrerequisites()

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotImplemented)
}

func TestGeminiAgent_ParseRateLimit_AlwaysReturnsFalse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
	}{
		{name: "empty output", output: ""},
		{name: "rate limit phrase", output: "rate limit exceeded, try again in 30 seconds"},
		{name: "arbitrary output", output: "some agent output with no rate limit signal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewGeminiAgent(AgentConfig{})
			info, limited := g.ParseRateLimit(tt.output)
			assert.Nil(t, info)
			assert.False(t, limited)
		})
	}
}

func TestGeminiAgent_DryRunCommand_ReturnsPlaceholder(t *testing.T) {
	t.Parallel()

	g := NewGeminiAgent(AgentConfig{})
	cmd := g.DryRunCommand(RunOpts{Prompt: "implement feature X"})

	assert.Equal(t, "# gemini adapter not yet implemented (v2.1)", cmd)
}

func TestGeminiAgent_DryRunCommand_IgnoresOpts(t *testing.T) {
	t.Parallel()

	g := NewGeminiAgent(AgentConfig{Model: "gemini-pro"})

	// Different RunOpts should all produce the same placeholder.
	opts := []RunOpts{
		{},
		{Prompt: "some prompt"},
		{Model: "gemini-ultra", Effort: "high"},
		{PromptFile: "/tmp/prompt.md", AllowedTools: "bash"},
	}

	for _, opt := range opts {
		assert.Equal(t, "# gemini adapter not yet implemented (v2.1)", g.DryRunCommand(opt))
	}
}

// ---------------------------------------------------------------------------
// ErrNotImplemented sentinel error tests
// ---------------------------------------------------------------------------

func TestErrNotImplemented_IsDistinct(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, ErrNotImplemented)

	// Must not overlap with other sentinel errors in the package.
	assert.False(t, errors.Is(ErrNotImplemented, ErrNotFound))
	assert.False(t, errors.Is(ErrNotImplemented, ErrDuplicateName))
	assert.False(t, errors.Is(ErrNotImplemented, ErrInvalidName))
}

func TestErrNotImplemented_ErrorMessage(t *testing.T) {
	t.Parallel()

	assert.Contains(t, ErrNotImplemented.Error(), "not implemented")
	assert.Contains(t, ErrNotImplemented.Error(), "v2.1")
}

func TestGeminiAgent_CanRegisterInRegistry(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	g := NewGeminiAgent(AgentConfig{})

	err := r.Register(g)
	require.NoError(t, err)

	got, err := r.Get("gemini")
	require.NoError(t, err)
	assert.Equal(t, g, got)
}
