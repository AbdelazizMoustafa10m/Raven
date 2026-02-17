# T-028: Loop Recovery -- Rate-Limit Wait and Dirty-Tree

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-004, T-005, T-015, T-025, T-027 |
| Blocked By | T-015, T-027 |
| Blocks | T-029 |

## Goal
Implement the recovery mechanisms for the implementation loop: rate-limit wait with countdown timer display, dirty-tree detection and auto-stash recovery, and graceful error recovery after agent failures. These mechanisms ensure the loop can recover from common disruptions without human intervention, which is critical for unattended operation.

## Background
Per PRD Section 5.4, the implementation loop needs:
- **Rate-limit recovery**: "Displays countdown timer during wait. Automatically retries after rate-limit reset."
- **Dirty-tree recovery**: "Dirty-tree detection (uncommitted changes after agent run). Auto-stash and recovery on unexpected state."
- **Error recovery**: "Detects when no progress is being made (same task selected N times in a row)."

Per PRD Section 5.13, the GitClient (T-015) provides `HasUncommittedChanges`, `Stash`, and `StashPop` methods. The rate-limit coordinator (T-025) provides `WaitForReset`. This task creates the integration layer that connects these capabilities into the loop runner's recovery flow.

## Technical Specifications
### Implementation Approach
Create `internal/loop/recovery.go` containing recovery functions that the loop runner (T-027) calls at specific points in the iteration cycle. The rate-limit wait function displays a countdown timer to stderr and blocks until the reset time. The dirty-tree recovery function detects uncommitted changes, stashes them, and provides a mechanism to restore them later. Error recovery handles agent failures with configurable retry behavior.

### Key Components
- **RateLimitWait**: Blocks with countdown display until rate-limit resets
- **DirtyTreeRecovery**: Detects and stashes uncommitted changes
- **StashRestore**: Restores stashed changes after successful iteration
- **RecoveryEvent**: Events emitted during recovery for TUI consumption

### API/Interface Contracts
```go
// internal/loop/recovery.go
package loop

import (
    "context"
    "io"
    "time"
)

// RecoveryEventType identifies recovery event types.
type RecoveryEventType string

const (
    EventRateLimitCountdown RecoveryEventType = "rate_limit_countdown"
    EventRateLimitResuming  RecoveryEventType = "rate_limit_resuming"
    EventDirtyTreeDetected  RecoveryEventType = "dirty_tree_detected"
    EventStashCreated       RecoveryEventType = "stash_created"
    EventStashRestored      RecoveryEventType = "stash_restored"
    EventStashFailed        RecoveryEventType = "stash_failed"
    EventRecoveryError      RecoveryEventType = "recovery_error"
)

// RecoveryEvent is emitted during recovery operations.
type RecoveryEvent struct {
    Type      RecoveryEventType
    Message   string
    Remaining time.Duration // For countdown events
    Timestamp time.Time
}

// RateLimitWaiter handles rate-limit wait with countdown display.
type RateLimitWaiter struct {
    coordinator *agent.RateLimitCoordinator
    output      io.Writer // stderr for countdown display
    events      chan<- RecoveryEvent
    logger      interface{ Info(msg string, kv ...interface{}) }
}

// NewRateLimitWaiter creates a waiter that coordinates with the rate-limit system.
func NewRateLimitWaiter(
    coordinator *agent.RateLimitCoordinator,
    output io.Writer,
    events chan<- RecoveryEvent,
    logger interface{ Info(msg string, kv ...interface{}) },
) *RateLimitWaiter

// Wait blocks until the rate limit resets for the given agent,
// displaying a countdown timer to the output writer.
// Returns an error if the context is cancelled or max waits exceeded.
func (w *RateLimitWaiter) Wait(ctx context.Context, agentName string) error

// displayCountdown renders a countdown timer to the output writer.
// Updates every second: "Rate limited. Retrying in 45s..."
// The countdown is interruptible via context cancellation.
func (w *RateLimitWaiter) displayCountdown(ctx context.Context, duration time.Duration) error

// DirtyTreeRecovery handles uncommitted changes in the working tree.
type DirtyTreeRecovery struct {
    gitClient *git.Client
    events    chan<- RecoveryEvent
    logger    interface{ Info(msg string, kv ...interface{}); Warn(msg string, kv ...interface{}) }
}

// NewDirtyTreeRecovery creates a recovery handler for dirty working trees.
func NewDirtyTreeRecovery(
    gitClient *git.Client,
    events chan<- RecoveryEvent,
    logger interface{ Info(msg string, kv ...interface{}); Warn(msg string, kv ...interface{}) },
) *DirtyTreeRecovery

// CheckAndStash checks for uncommitted changes and stashes them if found.
// Returns true if changes were stashed, false if the tree was clean.
// The stash message includes the task ID for identification.
func (dtr *DirtyTreeRecovery) CheckAndStash(ctx context.Context, taskID string) (bool, error)

// RestoreStash pops the most recent stash entry.
// Should be called after a successful agent run to restore stashed changes.
func (dtr *DirtyTreeRecovery) RestoreStash(ctx context.Context) error

// EnsureCleanTree verifies the working tree is clean.
// If dirty, attempts to stash. Returns an error if stash fails.
// This is called before starting a new task to ensure a clean starting state.
func (dtr *DirtyTreeRecovery) EnsureCleanTree(ctx context.Context, taskID string) error

// AgentErrorRecovery handles agent execution failures.
type AgentErrorRecovery struct {
    maxConsecutiveErrors int
    consecutiveErrors    int
    logger               interface{ Warn(msg string, kv ...interface{}) }
}

// NewAgentErrorRecovery creates an error recovery handler.
func NewAgentErrorRecovery(maxConsecutiveErrors int) *AgentErrorRecovery

// RecordError records an agent error and returns whether the loop should continue.
// Returns false if consecutive error limit is reached.
func (aer *AgentErrorRecovery) RecordError(err error) bool

// RecordSuccess resets the consecutive error counter.
func (aer *AgentErrorRecovery) RecordSuccess()

// ShouldAbort returns true if too many consecutive errors have occurred.
func (aer *AgentErrorRecovery) ShouldAbort() bool
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| context | stdlib | Cancellation during wait |
| time | stdlib | Timer for countdown |
| io | stdlib | Writer interface for countdown display |
| fmt | stdlib | Countdown formatting |
| internal/agent (T-025) | - | RateLimitCoordinator |
| internal/git (T-015) | - | Git client for stash operations |
| charmbracelet/log | latest | Structured logging |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] RateLimitWaiter displays countdown timer updating every second
- [ ] RateLimitWaiter blocks until rate limit resets
- [ ] RateLimitWaiter returns immediately if no rate limit active
- [ ] RateLimitWaiter respects context cancellation (Ctrl+C during countdown)
- [ ] Countdown display clears/overwrites previous line (carriage return)
- [ ] DirtyTreeRecovery detects uncommitted changes via GitClient
- [ ] CheckAndStash creates stash with task-identifying message
- [ ] RestoreStash pops the most recent stash
- [ ] EnsureCleanTree stashes if dirty, no-op if clean
- [ ] Recovery events emitted for TUI consumption
- [ ] AgentErrorRecovery tracks consecutive errors
- [ ] AgentErrorRecovery.RecordError returns false after max consecutive errors
- [ ] AgentErrorRecovery.RecordSuccess resets the counter
- [ ] All recovery functions handle git command failures gracefully
- [ ] Unit tests achieve 85% coverage

## Testing Requirements
### Unit Tests
- RateLimitWaiter.Wait with 1-second rate limit: completes within 2 seconds
- RateLimitWaiter.Wait with cancelled context: returns context error immediately
- RateLimitWaiter.Wait with no active rate limit: returns immediately
- displayCountdown captures countdown text to writer
- CheckAndStash with dirty tree: returns true, stash created
- CheckAndStash with clean tree: returns false, no stash
- RestoreStash after stash: changes restored
- RestoreStash with no stash: returns error (or no-op)
- EnsureCleanTree with dirty tree: stashes, tree now clean
- EnsureCleanTree with clean tree: no-op
- AgentErrorRecovery with 3 errors and max=3: ShouldAbort returns true
- AgentErrorRecovery with 2 errors and max=3: ShouldAbort returns false
- AgentErrorRecovery with error then success: counter resets
- Recovery events emitted at correct points

### Integration Tests
- Full recovery cycle: dirty tree -> stash -> agent run (mock) -> restore stash
- Rate-limit wait with mock coordinator: verify timing

### Edge Cases to Handle
- Stash fails (merge conflicts, no changes to stash): clear error message
- StashPop fails (conflicts): log warning, do not abort loop
- Rate-limit countdown interrupted at exactly 0: handle gracefully
- Rate-limit reset time already passed: return immediately
- Git not available: dirty-tree recovery returns clear error
- Git stash when already in stash state (nested stash): handle safely
- Output writer is nil (TUI mode where countdown is displayed differently): skip display
- Rate limit re-detected during countdown (new, longer limit): extend wait

## Implementation Notes
### Recommended Approach
1. Implement RateLimitWaiter first -- it's used most frequently
2. displayCountdown: use `time.NewTicker(1 * time.Second)` for updates, select on ticker.C and ctx.Done()
3. Display format: `"\rRate limited. Retrying in %ds...  "` (carriage return to overwrite line, trailing spaces to clear shorter strings)
4. After countdown: print newline to move past the countdown line
5. DirtyTreeRecovery: call gitClient.HasUncommittedChanges(), if true call gitClient.Stash("raven-autostash: " + taskID)
6. RestoreStash: call gitClient.StashPop()
7. EnsureCleanTree: check + stash in one call, used at loop start
8. AgentErrorRecovery: simple counter with threshold
9. All recovery functions should log at appropriate levels (info for stash operations, debug for countdown ticks)

### Potential Pitfalls
- The countdown display must use carriage return (`\r`) not newline for in-place updates -- but only in terminal mode. If output is piped, use simple line-by-line output
- Stash operations can fail if there are conflicts or if the working directory has untracked files that conflict -- catch these errors and report clearly
- The rate-limit countdown timer should use the coordinator's remaining wait time, not a fixed duration -- the actual wait may be shorter if time has passed since detection
- Do not hold any locks while displaying the countdown (the coordinator uses RWMutex)
- Git stash creates a stash entry even if there are only untracked files -- be aware of `--include-untracked` behavior

### Security Considerations
- Stash operations may contain sensitive code changes -- log stash messages at debug level
- Git stash entries persist in the repository -- document that Raven creates auto-stash entries

## References
- [PRD Section 5.4 - Rate-limit recovery](docs/prd/PRD-Raven.md)
- [PRD Section 5.4 - Dirty-tree recovery](docs/prd/PRD-Raven.md)
- [PRD Section 5.13 - Git Integration](docs/prd/PRD-Raven.md)
- [Go time.Ticker for periodic operations](https://pkg.go.dev/time#Ticker)
