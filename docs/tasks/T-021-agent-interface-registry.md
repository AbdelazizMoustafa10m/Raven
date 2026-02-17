# T-021: Agent Interface and Agent Registry

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-004, T-005, T-009 |
| Blocked By | T-004 |
| Blocks | T-022, T-023, T-024, T-025, T-027, T-035, T-038, T-039 |

## Goal
Define the Agent interface, the agent Registry for looking up agents by name, and the mock agent implementation for testing. This is the foundational abstraction for the entire agent adapter system -- every agent (Claude, Codex, Gemini) implements this interface, and every consumer (implementation loop, review pipeline, fix engine, PR generator) uses the registry to obtain agent instances.

## Background
Per PRD Section 5.2, all agents implement a common interface with methods for running prompts, checking prerequisites, parsing rate limits, and generating dry-run command strings. The `RunOpts` and `RunResult` types are defined in T-004 (Central Data Types). The registry provides named lookup so that `--agent claude` resolves to the Claude adapter instance. Per PRD: "Agent selection via `--agent <name>` flag or `raven.toml` config per workflow." The registry also supports iteration for multi-agent scenarios (review pipeline fans out to multiple agents).

## Technical Specifications
### Implementation Approach
Create `internal/agent/agent.go` containing the Agent interface, the Registry type, and any shared utility functions. Create `internal/agent/mock.go` containing a MockAgent for testing. The Agent interface exactly matches the PRD specification. The Registry is a simple map-based lookup with registration and retrieval methods. Agent configuration from `raven.toml` (model, effort, allowed tools, etc.) is passed via the agent constructors, not stored in the registry.

### Key Components
- **Agent**: The core interface all agent adapters implement
- **Registry**: Named lookup map for agent instances
- **MockAgent**: Configurable mock for testing (supports canned responses, error simulation, rate-limit simulation)
- **AgentConfig**: Configuration subset from raven.toml for a specific agent

### API/Interface Contracts
```go
// internal/agent/agent.go
package agent

import (
    "context"
    "fmt"
    "time"
)

// Agent is the interface that all AI agent adapters must implement.
// It abstracts the differences between Claude, Codex, Gemini, and other
// agents behind a common contract for prompt execution and rate-limit handling.
type Agent interface {
    // Name returns the agent's identifier (e.g., "claude", "codex").
    Name() string

    // Run executes a prompt using the agent and returns the result.
    // The context is used for cancellation and timeout.
    Run(ctx context.Context, opts RunOpts) (*RunResult, error)

    // CheckPrerequisites verifies that the agent's CLI tool is installed
    // and accessible. Returns an error describing what is missing.
    CheckPrerequisites() error

    // ParseRateLimit examines agent output for rate-limit signals.
    // Returns rate-limit info and true if a rate limit was detected.
    ParseRateLimit(output string) (*RateLimitInfo, bool)

    // DryRunCommand returns the command string that would be executed,
    // without actually running it. Used for --dry-run mode.
    DryRunCommand(opts RunOpts) string
}

// RunOpts configures a single agent invocation.
// Defined in T-004 central types; re-exported here for convenience.
// Fields: Prompt, PromptFile, Model, Effort, AllowedTools, OutputFormat,
// WorkDir, Env.

// RunResult captures the output of a single agent invocation.
// Defined in T-004 central types; re-exported here for convenience.
// Fields: Stdout, Stderr, ExitCode, Duration, RateLimit.

// RateLimitInfo describes a rate-limit event detected in agent output.
// Defined in T-004 central types; re-exported here for convenience.
// Fields: IsLimited, ResetAfter, Message.

// AgentConfig holds agent-specific configuration from raven.toml.
type AgentConfig struct {
    Command        string `toml:"command"`
    Model          string `toml:"model"`
    Effort         string `toml:"effort"`
    PromptTemplate string `toml:"prompt_template"`
    AllowedTools   string `toml:"allowed_tools"`
}

// Registry stores named agent instances for lookup.
type Registry struct {
    agents map[string]Agent
}

// NewRegistry creates an empty agent registry.
func NewRegistry() *Registry

// Register adds an agent to the registry under its Name().
// Returns an error if an agent with the same name is already registered.
func (r *Registry) Register(agent Agent) error

// Get returns the agent registered under the given name.
// Returns an error if no agent is found.
func (r *Registry) Get(name string) (Agent, error)

// MustGet returns the agent or panics. Only use in init/setup code.
func (r *Registry) MustGet(name string) Agent

// List returns the names of all registered agents, sorted alphabetically.
func (r *Registry) List() []string

// Has returns true if an agent with the given name is registered.
func (r *Registry) Has(name string) bool
```

```go
// internal/agent/mock.go
package agent

import (
    "context"
    "time"
)

// MockAgent is a configurable mock implementation of Agent for testing.
type MockAgent struct {
    AgentName       string
    RunFunc         func(ctx context.Context, opts RunOpts) (*RunResult, error)
    PrereqError     error
    RateLimitResult *RateLimitInfo
    DryRunOutput    string
    Calls           []RunOpts // records all Run calls for verification
}

func (m *MockAgent) Name() string { return m.AgentName }

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

func (m *MockAgent) CheckPrerequisites() error {
    return m.PrereqError
}

func (m *MockAgent) ParseRateLimit(output string) (*RateLimitInfo, bool) {
    if m.RateLimitResult != nil {
        return m.RateLimitResult, m.RateLimitResult.IsLimited
    }
    return nil, false
}

func (m *MockAgent) DryRunCommand(opts RunOpts) string {
    if m.DryRunOutput != "" {
        return m.DryRunOutput
    }
    return fmt.Sprintf("mock-agent --prompt %q", opts.Prompt)
}

// NewMockAgent creates a MockAgent with the given name and default behavior.
func NewMockAgent(name string) *MockAgent

// WithRunFunc sets a custom Run function on the MockAgent.
func (m *MockAgent) WithRunFunc(fn func(ctx context.Context, opts RunOpts) (*RunResult, error)) *MockAgent

// WithRateLimit configures the mock to always detect rate limits.
func (m *MockAgent) WithRateLimit(resetAfter time.Duration) *MockAgent

// WithPrereqError configures the mock to fail prerequisite checks.
func (m *MockAgent) WithPrereqError(err error) *MockAgent
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| context | stdlib | Cancellation propagation |
| time | stdlib | Duration types |
| fmt | stdlib | Error formatting |
| sort | stdlib | Sorted agent name listing |
| sync | stdlib | Thread-safe registry (if needed) |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Agent interface matches PRD Section 5.2 specification exactly (5 methods)
- [ ] Registry supports Register, Get, MustGet, List, Has operations
- [ ] Register returns error on duplicate agent name
- [ ] Get returns descriptive error for unknown agent name
- [ ] List returns sorted agent names
- [ ] MockAgent implements Agent interface (compile-time check)
- [ ] MockAgent records all Run calls for test verification
- [ ] MockAgent supports custom RunFunc for scenario-specific behavior
- [ ] MockAgent supports rate-limit simulation
- [ ] MockAgent supports prerequisite error simulation
- [ ] AgentConfig struct matches raven.toml `[agents.*]` section
- [ ] All types and methods have doc comments
- [ ] Unit tests achieve 95% coverage

## Testing Requirements
### Unit Tests
- Registry: Register and Get round-trip works
- Registry: Duplicate registration returns error
- Registry: Get unknown name returns error
- Registry: List returns sorted names
- Registry: Has returns true for registered, false for unregistered
- MockAgent: implements Agent (type assertion)
- MockAgent: Run records calls
- MockAgent: default Run returns mock output
- MockAgent: custom RunFunc is called
- MockAgent: WithRateLimit configures rate-limit detection
- MockAgent: WithPrereqError configures prerequisite failure
- MockAgent: ParseRateLimit returns configured result
- MockAgent: DryRunCommand returns expected string

### Integration Tests
- Register 3 agents (mock), iterate over all, verify each produces output

### Edge Cases to Handle
- Empty agent name (reject at registration)
- Nil agent (reject at registration)
- Concurrent Get calls (Registry should be safe for concurrent reads)
- MustGet on unregistered agent (should panic with descriptive message)
- Registry with zero agents: List returns empty slice, Get always errors

## Implementation Notes
### Recommended Approach
1. Start with the Agent interface -- this must match PRD exactly
2. Implement Registry as a simple `map[string]Agent` with error-returning methods
3. Implement MockAgent with builder methods (WithRunFunc, WithRateLimit, etc.)
4. Add a compile-time interface check: `var _ Agent = (*MockAgent)(nil)`
5. Keep the registry simple for v2.0 -- no concurrent write safety needed (agents are registered at startup)
6. If concurrent reads during runtime are a concern, use `sync.RWMutex`
7. AgentConfig will be consumed by agent constructors (T-022, T-023, T-024) but defined here for shared access

### Potential Pitfalls
- The types RunOpts, RunResult, RateLimitInfo are defined in T-004. Decide whether to re-export from the agent package or import from a shared types package. Recommendation: import from the types package defined in T-004
- Do not put agent CLI logic in this file -- this is purely the interface and registry
- MustGet should only be used in initialization code, never in request paths

### Security Considerations
- Agent names are used to look up adapters -- validate that names contain only alphanumeric characters and hyphens
- The registry does not store credentials -- agent authentication is handled by the CLI tools themselves

## References
- [PRD Section 5.2 - Agent Adapter System](docs/prd/PRD-Raven.md)
- [PRD Section 6.4 - Central Data Types (RunOpts, RunResult)](docs/prd/PRD-Raven.md)
- [Go interface design best practices](https://go.dev/doc/effective_go#interfaces)
