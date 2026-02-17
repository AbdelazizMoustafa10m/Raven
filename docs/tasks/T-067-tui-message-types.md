# T-067: TUI Message Types and Event System

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-004, T-066 |
| Blocked By | T-066 |
| Blocks | T-070, T-071, T-072, T-073, T-074, T-075 |

## Goal
Define all custom `tea.Msg` types that flow through the Bubble Tea event loop, bridging the workflow engine, agent adapters, and implementation loop with the TUI presentation layer. These message types form the contract between backend goroutines (which produce events via `p.Send()`) and the TUI's `Update()` method (which consumes them to update the UI).

## Background
Per PRD Section 5.12 and Section 6.3, the TUI consumes events from the workflow engine and agent adapters via Go channels. In Bubble Tea's architecture, external goroutines communicate with the TUI through `tea.Program.Send(msg)`, where `msg` satisfies `tea.Msg` (which is `interface{}`). The TUI's `Update()` method type-switches on incoming messages to route them to the appropriate sub-model.

Three primary event sources feed the TUI:
1. **Agent adapters** -- produce `AgentOutputMsg` for each line of agent stdout/stderr, status changes, and rate-limit signals
2. **Workflow engine** -- produces `WorkflowEventMsg` for step transitions, checkpoint events, and workflow lifecycle changes
3. **Implementation loop** -- produces `LoopEventMsg` for iteration counts, task selection, wait timers, and completion signals

Additionally, the TUI needs internal messages for UI-specific concerns like tick timers (for countdown displays) and focus changes.

## Technical Specifications
### Implementation Approach
Create `internal/tui/messages.go` containing all custom message types as plain Go structs. Each message type embeds enough information for the TUI to update its state without needing access to the underlying systems. Messages are immutable value types (no pointers to mutable state). Include a `TickMsg` for periodic UI updates (rate-limit countdowns, elapsed timers).

### Key Components
- **AgentOutputMsg**: A single line of output from an agent, tagged with agent name and stream (stdout/stderr)
- **AgentStatusMsg**: Agent lifecycle changes (started, completed, failed, rate-limited)
- **WorkflowEventMsg**: Workflow step transitions and state changes
- **LoopEventMsg**: Implementation loop iteration events
- **RateLimitMsg**: Rate-limit detection with countdown information
- **TickMsg**: Periodic timer tick for countdown displays and elapsed time
- **TaskProgressMsg**: Task completion state changes for progress bar updates
- **ErrorMsg**: Non-fatal errors that should be displayed in the event log

### API/Interface Contracts
```go
// internal/tui/messages.go

import "time"

// --- Agent Messages ---

// AgentOutputMsg represents a single line of output from an agent process.
type AgentOutputMsg struct {
    Agent     string    // Agent name (e.g., "claude", "codex")
    Line      string    // The output line
    Stream    string    // "stdout" or "stderr"
    Timestamp time.Time // When the line was produced
}

// AgentStatus represents the current state of an agent.
type AgentStatus int

const (
    AgentIdle AgentStatus = iota
    AgentRunning
    AgentCompleted
    AgentFailed
    AgentRateLimited
    AgentWaiting
)

// AgentStatusMsg signals an agent lifecycle change.
type AgentStatusMsg struct {
    Agent     string
    Status    AgentStatus
    Task      string    // Current task ID (e.g., "T-007")
    Detail    string    // Human-readable detail (e.g., "Working on auth middleware")
    Timestamp time.Time
}

// --- Workflow Messages ---

// WorkflowEventMsg signals a workflow engine state change.
type WorkflowEventMsg struct {
    WorkflowID   string
    WorkflowName string
    Step         string    // Current step name
    PrevStep     string    // Previous step name (empty on start)
    Event        string    // Transition event (success, failure, rate_limited, etc.)
    Detail       string    // Human-readable detail
    Timestamp    time.Time
}

// --- Loop Messages ---

// LoopEventMsg signals an implementation loop event.
type LoopEventMsg struct {
    Type      LoopEventType
    TaskID    string        // Current task being worked on
    Iteration int           // Current iteration number
    MaxIter   int           // Maximum iterations configured
    Detail    string        // Human-readable detail
    Timestamp time.Time
}

// LoopEventType categorizes implementation loop events.
type LoopEventType int

const (
    LoopIterationStarted LoopEventType = iota
    LoopIterationCompleted
    LoopTaskSelected
    LoopTaskCompleted
    LoopTaskBlocked
    LoopWaitingForRateLimit
    LoopResumedAfterWait
    LoopPhaseComplete
    LoopError
)

// --- Rate Limit Messages ---

// RateLimitMsg signals a rate-limit event with countdown.
type RateLimitMsg struct {
    Provider   string        // Provider name (e.g., "anthropic", "openai")
    Agent      string        // Agent that triggered the rate limit
    ResetAfter time.Duration // Time until rate limit resets
    ResetAt    time.Time     // Absolute time when rate limit resets
    Timestamp  time.Time
}

// --- Task Progress Messages ---

// TaskProgressMsg signals a task state change for progress tracking.
type TaskProgressMsg struct {
    TaskID      string
    TaskTitle   string
    Status      string // not_started, in_progress, completed, blocked, skipped
    Phase       int
    Completed   int    // Total completed tasks
    Total       int    // Total tasks
    Timestamp   time.Time
}

// --- Internal TUI Messages ---

// TickMsg is sent periodically for timer updates (countdowns, elapsed time).
type TickMsg struct {
    Time time.Time
}

// ErrorMsg represents a non-fatal error to display in the event log.
type ErrorMsg struct {
    Source  string // Component that produced the error
    Detail  string
    Timestamp time.Time
}

// FocusChangedMsg signals that keyboard focus moved to a different panel.
type FocusChangedMsg struct {
    Panel FocusPanel
}

// --- Helper Functions ---

// TickCmd returns a tea.Cmd that sends a TickMsg after the given duration.
func TickCmd(d time.Duration) tea.Cmd

// TickEvery returns a tea.Cmd that sends periodic TickMsg at the given interval.
// Used for rate-limit countdowns and elapsed time displays.
func TickEvery(d time.Duration) tea.Cmd
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/bubbletea | v1.2+ | tea.Msg, tea.Cmd types |
| time | stdlib | Timestamps and durations |

## Acceptance Criteria
- [ ] All message types are defined in `internal/tui/messages.go`
- [ ] All message types satisfy `tea.Msg` (which is `interface{}` -- all types satisfy it)
- [ ] Each message carries a `Timestamp` field for event log ordering
- [ ] `AgentStatus` enum covers all agent lifecycle states from PRD
- [ ] `LoopEventType` enum covers all loop events from PRD Section 5.4
- [ ] `TickCmd` returns a `tea.Cmd` that sends a `TickMsg` after the specified duration
- [ ] `TickEvery` returns a repeating tick command
- [ ] String methods on enum types for human-readable display
- [ ] Unit tests cover message construction and enum string conversion
- [ ] All types are documented with Go doc comments

## Testing Requirements
### Unit Tests
- All message types can be constructed with literal syntax
- `AgentStatus.String()` returns expected values for all enum variants
- `LoopEventType.String()` returns expected values for all enum variants
- `TickCmd` with 1-second duration returns a valid `tea.Cmd`
- Type assertion: messages can be type-switched in a simulated Update function
- Timestamp fields default to zero value (caller sets them)

### Integration Tests
- Simulate sending each message type through a channel and receiving it in a type switch

### Edge Cases to Handle
- Empty agent name or task ID in messages (should not panic, just display empty)
- Zero-value `time.Duration` in `RateLimitMsg` (means unknown reset time)
- Very long output lines in `AgentOutputMsg` (>2000 chars) -- TUI will truncate in View, not here

## Implementation Notes
### Recommended Approach
1. Define all message types as exported structs in `messages.go`
2. Define enum types (`AgentStatus`, `LoopEventType`) with `iota` constants
3. Add `String()` methods on enum types using a string slice lookup
4. Implement `TickCmd` using `tea.Tick(d, func(t time.Time) tea.Msg { return TickMsg{Time: t} })`
5. Implement `TickEvery` similarly but have the returned Msg trigger another tick
6. Add comprehensive doc comments for each type explaining its purpose and lifecycle

### Potential Pitfalls
- `tea.Tick` is the idiomatic way to schedule timed messages in Bubble Tea. Do NOT use `time.After` in a goroutine -- that bypasses the Elm architecture and can cause races.
- Messages should be value types (not pointers) to avoid any possibility of mutation after sending.
- The `TickEvery` implementation must return a new tick command from the `Update` handler when it receives a `TickMsg`, creating a recursive scheduling pattern. The initial `TickCmd` should be returned from `Init()` or when a countdown starts.
- Do not import any `internal/workflow`, `internal/agent`, or `internal/loop` packages here. The TUI message types are a separate, independent contract. The adapter code that converts engine events to TUI messages lives in the `raven dashboard` command (T-078).

### Security Considerations
- Agent output lines may contain sensitive information (API keys, tokens). The TUI message system passes them through as-is; sanitization is the responsibility of the view layer.

## References
- [Bubble Tea Commands Tutorial](https://github.com/charmbracelet/bubbletea/blob/main/tutorials/commands/README.md)
- [Bubble Tea send-msg Example](https://github.com/charmbracelet/bubbletea/blob/main/examples/send-msg/main.go)
- [PRD Section 5.12 - Live Updates via Go Channels](docs/prd/PRD-Raven.md)
- [PRD Section 6.3 - Concurrency Model](docs/prd/PRD-Raven.md)