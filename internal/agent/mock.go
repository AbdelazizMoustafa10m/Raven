package agent

import (
	"context"
	"fmt"
	"time"
)

// Compile-time check that MockAgent implements Agent.
var _ Agent = (*MockAgent)(nil)

// MockAgent is a configurable mock implementation of Agent for testing.
// It records all Run calls and supports customizable behavior via builder
// methods and function fields.
type MockAgent struct {
	// AgentName is the value returned by Name().
	AgentName string

	// RunFunc is an optional custom function called by Run. If nil, Run
	// returns a default success result with "mock output".
	RunFunc func(ctx context.Context, opts RunOpts) (*RunResult, error)

	// PrereqError is the error returned by CheckPrerequisites. A nil value
	// means prerequisites are satisfied.
	PrereqError error

	// RateLimitResult is the *RateLimitInfo returned by ParseRateLimit.
	// When non-nil, ParseRateLimit returns this value and RateLimitResult.IsLimited.
	RateLimitResult *RateLimitInfo

	// DryRunOutput is the string returned by DryRunCommand. When empty, a
	// default format is used.
	DryRunOutput string

	// Calls records every set of RunOpts passed to Run, in order. Tests can
	// inspect this slice to verify call count and arguments.
	Calls []RunOpts
}

// NewMockAgent creates a MockAgent with the given name and default behavior.
// By default: Run returns "mock output", CheckPrerequisites succeeds, and
// ParseRateLimit reports no rate limit.
func NewMockAgent(name string) *MockAgent {
	return &MockAgent{AgentName: name}
}

// Name returns the agent's identifier.
func (m *MockAgent) Name() string {
	return m.AgentName
}

// Run records the call and delegates to RunFunc if set, otherwise returns a
// default success result.
func (m *MockAgent) Run(ctx context.Context, opts RunOpts) (*RunResult, error) {
	m.Calls = append(m.Calls, opts)
	if m.RunFunc != nil {
		return m.RunFunc(ctx, opts)
	}
	return &RunResult{
		Stdout:   "mock output",
		ExitCode: 0,
		Duration: 100 * time.Millisecond,
	}, nil
}

// CheckPrerequisites returns PrereqError, which is nil by default (success).
func (m *MockAgent) CheckPrerequisites() error {
	return m.PrereqError
}

// ParseRateLimit returns RateLimitResult and its IsLimited field when
// RateLimitResult is non-nil. Otherwise returns nil and false.
func (m *MockAgent) ParseRateLimit(output string) (*RateLimitInfo, bool) {
	if m.RateLimitResult != nil {
		return m.RateLimitResult, m.RateLimitResult.IsLimited
	}
	return nil, false
}

// DryRunCommand returns DryRunOutput when set, otherwise a formatted default.
func (m *MockAgent) DryRunCommand(opts RunOpts) string {
	if m.DryRunOutput != "" {
		return m.DryRunOutput
	}
	return fmt.Sprintf("mock-agent --prompt %q", opts.Prompt)
}

// WithRunFunc sets a custom Run function on the MockAgent and returns the
// receiver for method chaining.
func (m *MockAgent) WithRunFunc(fn func(ctx context.Context, opts RunOpts) (*RunResult, error)) *MockAgent {
	m.RunFunc = fn
	return m
}

// WithRateLimit configures the mock to always detect a rate limit with the
// given reset duration. Returns the receiver for method chaining.
func (m *MockAgent) WithRateLimit(resetAfter time.Duration) *MockAgent {
	m.RateLimitResult = &RateLimitInfo{
		IsLimited:  true,
		ResetAfter: resetAfter,
		Message:    "mock rate limit",
	}
	return m
}

// WithPrereqError configures the mock to return the given error from
// CheckPrerequisites. Returns the receiver for method chaining.
func (m *MockAgent) WithPrereqError(err error) *MockAgent {
	m.PrereqError = err
	return m
}
