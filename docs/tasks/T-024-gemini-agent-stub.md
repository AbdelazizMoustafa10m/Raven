# T-024: Gemini Agent Adapter Stub

## Metadata
| Field | Value |
|-------|-------|
| Priority | Should Have |
| Estimated Effort | Small: 2-3hrs |
| Dependencies | T-004, T-021 |
| Blocked By | T-021 |
| Blocks | None |

## Goal
Implement a stub adapter for the Gemini AI agent that satisfies the Agent interface but returns `ErrNotImplemented` for execution methods. This ensures the agent registry is complete, allows `--agent gemini` to produce a clear error message, and provides the scaffold for full Gemini support in v2.1.

## Background
Per PRD Section 5.2, Gemini is listed as a built-in adapter with the note: "Adapter stub for future integration. Implements the interface with `ErrNotImplemented` for unsupported features." The full Gemini adapter is scheduled for v2.1 (post-release). The stub must be registerable in the agent registry so that `raven implement --agent gemini` produces a user-friendly message ("Gemini adapter is not yet implemented. See v2.1 roadmap.") rather than an "unknown agent" error.

## Technical Specifications
### Implementation Approach
Create `internal/agent/gemini.go` containing a minimal `GeminiAgent` struct that implements the Agent interface. `Run()` returns `ErrNotImplemented`. `CheckPrerequisites()` returns `ErrNotImplemented`. `ParseRateLimit()` returns false (no rate-limit parsing). `DryRunCommand()` returns a placeholder string.

### Key Components
- **GeminiAgent**: Stub implementation of Agent interface
- **ErrNotImplemented**: Sentinel error for unimplemented features

### API/Interface Contracts
```go
// internal/agent/gemini.go
package agent

import (
    "context"
    "errors"
)

// ErrNotImplemented is returned by agent methods that are not yet implemented.
var ErrNotImplemented = errors.New("not implemented: this agent adapter is a stub for future integration (see v2.1 roadmap)")

// GeminiAgent is a stub adapter for the Gemini AI agent.
// Full implementation is planned for v2.1.
type GeminiAgent struct {
    config AgentConfig
}

// NewGeminiAgent creates a Gemini agent stub.
func NewGeminiAgent(config AgentConfig) *GeminiAgent

// Compile-time interface check.
var _ Agent = (*GeminiAgent)(nil)

func (g *GeminiAgent) Name() string { return "gemini" }

// Run returns ErrNotImplemented.
func (g *GeminiAgent) Run(ctx context.Context, opts RunOpts) (*RunResult, error) {
    return nil, ErrNotImplemented
}

// CheckPrerequisites returns ErrNotImplemented.
func (g *GeminiAgent) CheckPrerequisites() error {
    return ErrNotImplemented
}

// ParseRateLimit returns false (no rate-limit parsing for stub).
func (g *GeminiAgent) ParseRateLimit(output string) (*RateLimitInfo, bool) {
    return nil, false
}

// DryRunCommand returns a placeholder indicating the adapter is not implemented.
func (g *GeminiAgent) DryRunCommand(opts RunOpts) string {
    return "# gemini adapter not yet implemented (v2.1)"
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| context | stdlib | Interface compliance |
| errors | stdlib | ErrNotImplemented sentinel |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] GeminiAgent implements the Agent interface (compile-time check)
- [ ] Name() returns "gemini"
- [ ] Run() returns nil result and ErrNotImplemented
- [ ] CheckPrerequisites() returns ErrNotImplemented
- [ ] ParseRateLimit() returns nil and false for any input
- [ ] DryRunCommand() returns a human-readable placeholder string
- [ ] ErrNotImplemented is a package-level sentinel error (testable with errors.Is)
- [ ] GeminiAgent can be registered in the agent Registry without error
- [ ] All methods have doc comments explaining stub status
- [ ] Unit tests achieve 100% coverage

## Testing Requirements
### Unit Tests
- GeminiAgent implements Agent (type assertion)
- Name() returns "gemini"
- Run() returns ErrNotImplemented (verify with errors.Is)
- CheckPrerequisites() returns ErrNotImplemented
- ParseRateLimit("any string") returns nil, false
- DryRunCommand returns non-empty string
- Register GeminiAgent in Registry: succeeds
- Get "gemini" from Registry after registration: returns the stub

### Integration Tests
- None needed for a stub

### Edge Cases to Handle
- Run() called with nil context: should still return ErrNotImplemented (not panic)
- Config with gemini settings: agent creates successfully even though it won't use them

## Implementation Notes
### Recommended Approach
1. Define ErrNotImplemented as a package-level var (not const, since errors must be pointers)
2. All methods are trivial one-liners
3. The stub is primarily valuable for the registry -- it means `--agent gemini` gives a clear error rather than "agent not found"
4. Keep the file small and well-documented as a template for the v2.1 full implementation
5. Place TODO comments indicating what the full implementation will need

### Potential Pitfalls
- Ensure ErrNotImplemented is exported -- it will be checked by callers with errors.Is()
- Do not make this error type implement unwrap or custom error types -- keep it simple
- The stub should not import heavy packages (os/exec, etc.) -- it should be zero-cost

### Security Considerations
- No security concerns for a stub

## References
- [PRD Section 5.2 - Gemini adapter stub](docs/prd/PRD-Raven.md)
- [PRD v2.1 Roadmap - Full Gemini adapter](docs/prd/PRD-Raven.md)
