package agent

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by agent methods that are not yet implemented.
// It is used by stub adapters (e.g. GeminiAgent) to signal that the adapter
// exists for interface compliance but has no functional implementation.
// Full implementation is planned for v2.1.
var ErrNotImplemented = errors.New("not implemented: this agent adapter is a stub for future integration (see v2.1 roadmap)")

// GeminiAgent is a stub adapter for the Gemini AI agent.
// It satisfies the Agent interface so that "gemini" can be registered in the
// Registry and referenced in configuration, but all execution methods return
// ErrNotImplemented. Full implementation is planned for v2.1.
type GeminiAgent struct {
	config AgentConfig
}

// NewGeminiAgent creates a Gemini agent stub with the given configuration.
func NewGeminiAgent(config AgentConfig) *GeminiAgent {
	return &GeminiAgent{config: config}
}

// Compile-time interface check: GeminiAgent must satisfy Agent.
var _ Agent = (*GeminiAgent)(nil)

// Name returns the agent identifier "gemini".
func (g *GeminiAgent) Name() string { return "gemini" }

// Run returns ErrNotImplemented. The Gemini adapter is a stub; no subprocess
// is started and no output is produced.
func (g *GeminiAgent) Run(_ context.Context, _ RunOpts) (*RunResult, error) {
	return nil, ErrNotImplemented
}

// CheckPrerequisites returns ErrNotImplemented. Prerequisite checking is not
// available until the full adapter is implemented in v2.1.
func (g *GeminiAgent) CheckPrerequisites() error {
	return ErrNotImplemented
}

// ParseRateLimit always returns nil and false. The stub adapter does not
// inspect output or track rate-limit state.
func (g *GeminiAgent) ParseRateLimit(_ string) (*RateLimitInfo, bool) {
	return nil, false
}

// DryRunCommand returns a placeholder string indicating the adapter has not
// yet been implemented.
func (g *GeminiAgent) DryRunCommand(_ RunOpts) string {
	return "# gemini adapter not yet implemented (v2.1)"
}
