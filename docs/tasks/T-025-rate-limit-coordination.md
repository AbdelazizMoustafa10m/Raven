# T-025: Rate-Limit Detection and Coordination

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-004, T-021 |
| Blocked By | T-021 |
| Blocks | T-027, T-028 |

## Goal
Implement the shared rate-limit detection and coordination system that tracks rate-limit state across agents sharing the same provider. When one Claude agent hits a rate limit, all Claude-based agents should pause. This prevents wasted API calls and coordinates backoff across concurrent agents in the review pipeline and multi-phase workflows.

## Background
Per PRD Section 5.2, "Rate-limit coordination: when one agent hits a rate limit, all agents for the same provider pause (shared rate-limit state)." This is critical for the review pipeline (T-035) where multiple agents may share the same API provider, and for the implementation loop (T-027) where rate-limit detection triggers a wait-and-retry cycle. The coordination layer sits between the agent adapters and the loop/orchestrator, providing a centralized view of rate-limit state.

Per PRD Section 5.4, rate-limit recovery includes: detecting rate-limit signals in agent output, computing wait time from agent output or using configurable default backoff, displaying countdown timer during wait, and automatically retrying after rate-limit reset. Configurable `--max-limit-waits <n>` caps wait cycles.

## Technical Specifications
### Implementation Approach
Create `internal/agent/ratelimit.go` containing a `RateLimitCoordinator` that maintains per-provider rate-limit state. When an agent reports a rate limit, the coordinator records it and computes the earliest safe retry time. Other agents querying the same provider check this state before attempting execution. The coordinator is thread-safe for use by concurrent goroutines (review pipeline workers).

### Key Components
- **RateLimitCoordinator**: Central coordinator for rate-limit state across agents
- **ProviderState**: Per-provider rate-limit tracking (reset time, wait count)
- **BackoffConfig**: Configurable backoff parameters (default wait, max waits)
- **Provider mapping**: Maps agent names to provider names (claude -> anthropic, codex -> openai)

### API/Interface Contracts
```go
// internal/agent/ratelimit.go
package agent

import (
    "context"
    "sync"
    "time"
)

// Provider names for rate-limit grouping.
const (
    ProviderAnthropic = "anthropic"
    ProviderOpenAI    = "openai"
    ProviderGoogle    = "google"
)

// AgentProvider maps agent names to their API provider.
// Agents sharing a provider share rate-limit state.
var AgentProvider = map[string]string{
    "claude": ProviderAnthropic,
    "codex":  ProviderOpenAI,
    "gemini": ProviderGoogle,
}

// BackoffConfig configures rate-limit backoff behavior.
type BackoffConfig struct {
    DefaultWait  time.Duration // Default wait when reset time is unknown (default: 60s)
    MaxWaits     int           // Maximum number of rate-limit waits before aborting (default: 5)
    JitterFactor float64       // Random jitter factor 0.0-1.0 (default: 0.1)
}

// DefaultBackoffConfig returns sensible default backoff configuration.
func DefaultBackoffConfig() BackoffConfig

// ProviderState tracks rate-limit state for a single API provider.
type ProviderState struct {
    Provider    string
    IsLimited   bool
    ResetAt     time.Time     // When the rate limit resets
    ResetAfter  time.Duration // Original duration from agent output
    WaitCount   int           // Number of times we've waited for this provider
    LastMessage string        // Last rate-limit message for display
    UpdatedAt   time.Time     // When this state was last updated
}

// RemainingWait returns the time remaining until the rate limit resets.
// Returns 0 if the rate limit has already reset.
func (ps *ProviderState) RemainingWait() time.Duration

// RateLimitCoordinator manages rate-limit state across all providers.
// It is safe for concurrent use by multiple goroutines.
type RateLimitCoordinator struct {
    mu       sync.RWMutex
    states   map[string]*ProviderState // provider name -> state
    config   BackoffConfig
    onUpdate func(ProviderState) // optional callback for TUI notifications
}

// NewRateLimitCoordinator creates a coordinator with the given backoff config.
func NewRateLimitCoordinator(config BackoffConfig) *RateLimitCoordinator

// SetUpdateCallback sets a function called whenever rate-limit state changes.
// Used by the TUI to receive real-time rate-limit notifications.
func (rlc *RateLimitCoordinator) SetUpdateCallback(fn func(ProviderState))

// RecordRateLimit records that an agent hit a rate limit.
// It updates the provider's state and increments the wait count.
// Returns the ProviderState after recording.
func (rlc *RateLimitCoordinator) RecordRateLimit(agentName string, info *RateLimitInfo) *ProviderState

// ClearRateLimit clears the rate-limit state for a provider.
// Called after a successful agent run following a rate-limit wait.
func (rlc *RateLimitCoordinator) ClearRateLimit(agentName string)

// ShouldWait checks if an agent should wait before making a request.
// Returns the ProviderState if waiting is needed, nil if clear to proceed.
func (rlc *RateLimitCoordinator) ShouldWait(agentName string) *ProviderState

// WaitForReset blocks until the rate limit for the agent's provider resets
// or the context is cancelled. Returns an error if context is cancelled
// or max waits exceeded.
func (rlc *RateLimitCoordinator) WaitForReset(ctx context.Context, agentName string) error

// ExceededMaxWaits returns true if the provider has hit the max wait limit.
func (rlc *RateLimitCoordinator) ExceededMaxWaits(agentName string) bool

// GetState returns the current rate-limit state for a provider.
// Returns nil if no state exists for the agent's provider.
func (rlc *RateLimitCoordinator) GetState(agentName string) *ProviderState

// AllStates returns a snapshot of all provider states.
// Used by the TUI for the rate-limit status panel.
func (rlc *RateLimitCoordinator) AllStates() []ProviderState

// computeWaitDuration determines how long to wait based on rate-limit info
// and backoff configuration. Uses info.ResetAfter if available, otherwise
// falls back to config.DefaultWait with optional jitter.
func (rlc *RateLimitCoordinator) computeWaitDuration(info *RateLimitInfo) time.Duration
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| sync | stdlib | RWMutex for thread-safe state |
| time | stdlib | Duration, Timer for wait |
| context | stdlib | Cancellation during wait |
| math/rand | stdlib | Jitter calculation |
| fmt | stdlib | Error wrapping |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] RateLimitCoordinator is thread-safe for concurrent access
- [ ] RecordRateLimit updates provider state and increments wait count
- [ ] ClearRateLimit resets provider state after successful run
- [ ] ShouldWait returns state when provider is rate-limited, nil when clear
- [ ] ShouldWait checks the shared provider (claude and claude-2 share anthropic state)
- [ ] WaitForReset blocks until reset time passes
- [ ] WaitForReset respects context cancellation (Ctrl+C during wait)
- [ ] ExceededMaxWaits returns true after max waits reached
- [ ] AllStates returns snapshot of all provider states (for TUI)
- [ ] Agent-to-provider mapping is correct (claude->anthropic, codex->openai, gemini->google)
- [ ] Default backoff used when agent does not report specific reset duration
- [ ] Jitter applied to avoid thundering herd after rate-limit reset
- [ ] SetUpdateCallback notifies TUI when state changes
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- RecordRateLimit for claude: anthropic provider state updated
- RecordRateLimit with specific ResetAfter: ResetAt computed correctly
- RecordRateLimit with no ResetAfter: DefaultWait used
- ShouldWait after RecordRateLimit: returns non-nil state
- ShouldWait after reset time passed: returns nil
- ShouldWait for codex when claude is limited: returns nil (different provider)
- ClearRateLimit resets state: ShouldWait returns nil
- WaitForReset with short wait: completes without error
- WaitForReset with context cancelled: returns context error
- ExceededMaxWaits after N waits: returns true
- ExceededMaxWaits before N waits: returns false
- AllStates returns snapshot of all providers
- SetUpdateCallback: callback invoked on RecordRateLimit
- computeWaitDuration with jitter: result varies within expected range
- Concurrent RecordRateLimit and ShouldWait: no race conditions (run with -race)
- ProviderState.RemainingWait: correct remaining time
- ProviderState.RemainingWait when expired: returns 0

### Integration Tests
- Two goroutines: one records rate limit, other checks ShouldWait: coordination works
- Full cycle: record -> wait -> clear -> proceed

### Edge Cases to Handle
- Unknown agent name: default to agent name as provider (conservative grouping)
- Zero ResetAfter: use DefaultWait
- Negative ResetAfter: treat as zero, use DefaultWait
- Very long ResetAfter (>1 hour): cap at reasonable maximum or warn
- Rate limit recorded during WaitForReset: extends wait to new reset time
- Multiple agents for same provider hitting rate limits simultaneously: use latest reset time
- MaxWaits set to 0: no waits allowed, always returns ExceededMaxWaits
- Provider state accessed after coordinator is not used anymore: safe for GC

## Implementation Notes
### Recommended Approach
1. Define BackoffConfig and ProviderState structs first
2. Implement RateLimitCoordinator with sync.RWMutex for thread safety
3. RecordRateLimit: lock, update/create provider state, compute ResetAt = now + duration, increment WaitCount, unlock, call onUpdate callback
4. ShouldWait: read lock, check if provider state exists and ResetAt is in the future
5. WaitForReset: compute remaining time, create `time.Timer`, select on timer.C and ctx.Done()
6. ClearRateLimit: lock, reset provider state (set IsLimited=false, keep WaitCount for ExceededMaxWaits check)
7. Jitter: `duration + time.Duration(rand.Float64() * jitterFactor * float64(duration))`
8. Use `t.Parallel()` and `-race` flag in tests to verify concurrency safety

### Potential Pitfalls
- The update callback (for TUI) must not block -- if it does, it can deadlock the coordinator. Either use a buffered channel or make the callback non-blocking
- When computing ResetAt, use `time.Now().Add(duration)` -- do not use the agent's reported wall-clock time (clock skew between agent and Raven)
- The WaitForReset method must handle the case where the reset time has already passed by the time it's called (return immediately)
- Be careful with RWMutex: ClearRateLimit and RecordRateLimit need write locks, ShouldWait and GetState need read locks
- Do not hold the lock while calling the update callback (potential deadlock with TUI)

### Security Considerations
- Rate-limit state is ephemeral (in-memory only) -- no sensitive data persisted
- Rate-limit messages may contain organization identifiers -- only log at debug level

## References
- [PRD Section 5.2 - Rate-limit coordination](docs/prd/PRD-Raven.md)
- [PRD Section 5.4 - Rate-limit detection and recovery](docs/prd/PRD-Raven.md)
- [Go sync.RWMutex documentation](https://pkg.go.dev/sync#RWMutex)
- [Rate limiting patterns in distributed systems](https://cloud.google.com/architecture/rate-limiting-strategies-techniques)
