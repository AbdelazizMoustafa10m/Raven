# T-027: Implementation Loop Runner

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Large: 16-24hrs |
| Dependencies | T-004, T-005, T-009, T-015, T-016, T-017, T-019, T-021, T-025, T-026 |
| Blocked By | T-019, T-021, T-025, T-026 |
| Blocks | T-028, T-029, T-049 |

## Goal
Implement the core implementation loop engine that autonomously works through tasks: selecting the next task, generating a prompt, invoking the agent, detecting completion or rate limits, and iterating until the phase is complete or limits are reached. This is the workhorse of Raven -- the Go reimplementation of the bash prototype's `run_raven_loop` with proper error handling, structured events, and configurable limits.

## Background
Per PRD Section 5.4, the implementation loop "picks tasks, generates prompts, invokes agents, handles rate limits and errors, detects completion signals, and commits results." The loop iterates through tasks in a phase (or a single task), running the agent for each. It detects completion via signal strings in agent output (`PHASE_COMPLETE`, `TASK_BLOCKED`, `RAVEN_ERROR`), handles rate limits via the coordinator (T-025), and supports configurable limits (`--max-iterations`, `--max-limit-waits`, `--sleep`).

The loop is designed as a strategy-pattern composition: it takes a `TaskSelector`, `PromptGenerator`, and `Agent` as dependencies, making it testable with mocks. The loop emits `LoopEvent` messages for TUI consumption.

## Technical Specifications
### Implementation Approach
Create `internal/loop/runner.go` containing a `Runner` struct that orchestrates the implementation loop. The runner follows a clear iteration cycle: select task -> generate prompt -> check rate limits -> invoke agent -> scan output for signals -> update task state -> check termination conditions -> sleep -> repeat. Each phase of the iteration is a separate method for testability. The runner emits events at each stage via a channel.

### Key Components
- **Runner**: The main loop orchestrator
- **RunConfig**: Configuration for loop behavior (limits, sleep, dry-run)
- **LoopEvent**: Structured events emitted during loop execution
- **Signal detection**: Scans agent output for PHASE_COMPLETE, TASK_BLOCKED, RAVEN_ERROR
- **Iteration cycle**: The core select-prompt-run-detect-update loop

### API/Interface Contracts
```go
// internal/loop/runner.go
package loop

import (
    "context"
    "time"
)

// RunConfig configures the implementation loop behavior.
type RunConfig struct {
    AgentName     string        // Agent to use (e.g., "claude")
    PhaseID       int           // Phase to implement (0 for single-task mode)
    TaskID        string        // Specific task ID (empty for phase mode)
    MaxIterations int           // Maximum loop iterations (default: 50)
    MaxLimitWaits int           // Maximum rate-limit waits (default: 5)
    SleepBetween  time.Duration // Sleep between iterations (default: 5s)
    DryRun        bool          // Generate prompts without invoking agent
    TemplateName  string        // Prompt template to use (from agent config)
}

// LoopEvent represents a structured event emitted during loop execution.
type LoopEvent struct {
    Type       LoopEventType
    Iteration  int
    TaskID     string
    AgentName  string
    Message    string
    Timestamp  time.Time
    Duration   time.Duration // Duration of agent run (for agent_completed)
    WaitTime   time.Duration // Wait duration (for rate_limit_wait)
}

// LoopEventType identifies the type of loop event.
type LoopEventType string

const (
    EventLoopStarted      LoopEventType = "loop_started"
    EventTaskSelected     LoopEventType = "task_selected"
    EventPromptGenerated  LoopEventType = "prompt_generated"
    EventAgentStarted     LoopEventType = "agent_started"
    EventAgentCompleted   LoopEventType = "agent_completed"
    EventAgentError       LoopEventType = "agent_error"
    EventRateLimitWait    LoopEventType = "rate_limit_wait"
    EventRateLimitResume  LoopEventType = "rate_limit_resume"
    EventTaskCompleted    LoopEventType = "task_completed"
    EventTaskBlocked      LoopEventType = "task_blocked"
    EventPhaseComplete    LoopEventType = "phase_complete"
    EventLoopError        LoopEventType = "loop_error"
    EventLoopAborted      LoopEventType = "loop_aborted"
    EventMaxIterations    LoopEventType = "max_iterations"
    EventSleeping         LoopEventType = "sleeping"
    EventDryRun           LoopEventType = "dry_run"
)

// CompletionSignal represents a signal detected in agent output.
type CompletionSignal string

const (
    SignalPhaseComplete CompletionSignal = "PHASE_COMPLETE"
    SignalTaskBlocked   CompletionSignal = "TASK_BLOCKED"
    SignalRavenError    CompletionSignal = "RAVEN_ERROR"
)

// Runner orchestrates the implementation loop.
type Runner struct {
    selector   *task.TaskSelector
    promptGen  *PromptGenerator
    agent      agent.Agent
    stateManager *task.StateManager
    rateLimiter  *agent.RateLimitCoordinator
    config     *config.Config
    events     chan<- LoopEvent
    logger     interface{ Info(msg string, kv ...interface{}); Debug(msg string, kv ...interface{}) }
}

// NewRunner creates an implementation loop runner with all dependencies.
func NewRunner(
    selector *task.TaskSelector,
    promptGen *PromptGenerator,
    ag agent.Agent,
    stateManager *task.StateManager,
    rateLimiter *agent.RateLimitCoordinator,
    cfg *config.Config,
    events chan<- LoopEvent,
    logger interface{ Info(msg string, kv ...interface{}); Debug(msg string, kv ...interface{}) },
) *Runner

// Run executes the implementation loop until completion or limits are reached.
// Returns nil on successful phase completion, error on abort.
func (r *Runner) Run(ctx context.Context, runCfg RunConfig) error

// RunSingleTask runs the loop for a specific task (--task T-007 mode).
func (r *Runner) RunSingleTask(ctx context.Context, runCfg RunConfig) error

// --- Internal methods (one per iteration phase) ---

// selectTask picks the next task based on mode (phase or single-task).
func (r *Runner) selectTask(runCfg RunConfig) (*task.ParsedTaskSpec, error)

// generatePrompt creates the agent prompt for the selected task.
func (r *Runner) generatePrompt(spec *task.ParsedTaskSpec, runCfg RunConfig) (string, error)

// invokeAgent runs the agent with the generated prompt.
func (r *Runner) invokeAgent(ctx context.Context, prompt string, runCfg RunConfig) (*agent.RunResult, error)

// detectSignals scans agent output for completion signals.
func (r *Runner) detectSignals(output string) (CompletionSignal, string)

// handleCompletion processes a detected completion signal.
func (r *Runner) handleCompletion(signal CompletionSignal, detail string, taskID string, agentName string) error

// emit sends a LoopEvent to the events channel (non-blocking).
func (r *Runner) emit(event LoopEvent)

// DetectSignals scans output for completion signal strings.
// Exported for testing.
func DetectSignals(output string) (CompletionSignal, string)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| context | stdlib | Cancellation propagation |
| time | stdlib | Sleep, duration tracking |
| strings | stdlib | Signal detection in output |
| fmt | stdlib | Error wrapping |
| internal/task (T-016,T-017,T-019) | - | Task selection and state |
| internal/agent (T-021) | - | Agent interface |
| internal/agent (T-025) | - | Rate-limit coordinator |
| internal/loop (T-026) | - | Prompt generation |
| internal/config (T-009) | - | Project configuration |
| charmbracelet/log | latest | Structured logging |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Run executes the full loop: select -> prompt -> agent -> detect -> update -> repeat
- [ ] Loop terminates when all tasks in phase are completed (SelectNext returns nil)
- [ ] Loop terminates when PHASE_COMPLETE signal detected in output
- [ ] Loop terminates when max iterations reached
- [ ] Loop terminates when context is cancelled (Ctrl+C)
- [ ] TASK_BLOCKED signal marks current task as blocked and moves to next
- [ ] RAVEN_ERROR signal stops the loop with error
- [ ] Rate-limit detected: delegates to coordinator, waits, retries
- [ ] Rate-limit max waits exceeded: loop aborts with error
- [ ] Task state updated to in_progress when selected, completed when done
- [ ] Sleep between iterations is configurable and cancellable
- [ ] DryRun mode generates and displays prompts without invoking agent
- [ ] RunSingleTask works for --task T-007 mode
- [ ] LoopEvents emitted at each stage for TUI consumption
- [ ] Stale task detection: same task selected N times in a row triggers warning
- [ ] Events channel is non-blocking (dropped events don't stall the loop)
- [ ] Unit tests achieve 80% coverage using mock agent

## Testing Requirements
### Unit Tests
- Run with 3 tasks, mock agent succeeds each: all 3 completed, loop ends
- Run with 1 task, mock agent outputs PHASE_COMPLETE: task completed, loop ends
- Run with 1 task, mock agent outputs TASK_BLOCKED: task marked blocked
- Run with 1 task, mock agent outputs RAVEN_ERROR: loop returns error
- Run with max_iterations=2: loop stops after 2 iterations
- Run with rate-limited agent (first call): waits and retries
- Run with rate-limited agent exceeding max waits: loop aborts
- Run with cancelled context: loop exits cleanly
- RunSingleTask with specific task: only that task is processed
- DryRun mode: prompt printed, agent not invoked
- DetectSignals("...PHASE_COMPLETE..."): returns SignalPhaseComplete
- DetectSignals("...TASK_BLOCKED: missing dep..."): returns SignalTaskBlocked with detail
- DetectSignals("normal output"): returns empty signal
- Sleep between iterations: timing verified (within tolerance)
- Events emitted in correct order: loop_started -> task_selected -> agent_started -> agent_completed -> task_completed
- Stale task detection: same task 3 times triggers warning event

### Integration Tests
- Full loop with mock agent that completes tasks and outputs PHASE_COMPLETE
- Full loop with mock agent that rate-limits once then succeeds
- Full loop with testdata tasks and state files

### Edge Cases to Handle
- No tasks available (phase already complete): loop exits immediately
- All remaining tasks blocked (dependency deadlock): loop exits with clear message
- Agent output empty (no signals): treat as normal completion, proceed to next task
- Agent exits with non-zero code but no rate-limit: treat as error, log, continue to next task
- Very fast agent (completes in <100ms): sleep between iterations still applied
- Events channel full (TUI not consuming fast enough): drop events, do not block
- Prompt generation fails: emit error event, skip to next task
- Task state file locked by another process: retry with backoff

## Implementation Notes
### Recommended Approach
1. Define RunConfig, LoopEvent types, and CompletionSignal constants first
2. Implement DetectSignals as a standalone function (easy to test)
3. Implement the Runner constructor with dependency injection
4. Implement the main Run method as a for loop with iteration counter
5. Each iteration: selectTask -> if nil, break (phase complete) -> generatePrompt -> check rate limits -> invokeAgent -> detectSignals -> handleCompletion -> update state -> sleep
6. Use `time.NewTimer(sleep)` with select on timer.C and ctx.Done() for cancellable sleep
7. Events: use a helper `emit` method that does non-blocking channel send (select with default to drop)
8. DryRun: skip invokeAgent, print prompt to stderr, emit dry_run event
9. Stale detection: track last 3 selected task IDs; if all same, emit warning

### Potential Pitfalls
- The events channel must be buffered or the emit method must be non-blocking -- a blocking send on a full channel will stall the entire loop
- Context cancellation must be checked at multiple points: before agent invocation, during sleep, during rate-limit wait
- When updating task state to `completed`, verify the agent actually made progress (commits, file changes) -- or trust the signal for v2.0
- The same task being selected repeatedly may indicate the agent is not making progress -- cap with stale detection, not just max iterations
- Do not call `os.Exit()` from the runner -- return errors to the CLI layer which handles exit codes

### Security Considerations
- Prompts passed to agents may contain sensitive code -- log at debug level only
- Agent output may contain sensitive data -- log at debug level only
- The runner inherits the caller's environment -- no additional privilege escalation

## References
- [PRD Section 5.4 - Implementation Loop Engine](docs/prd/PRD-Raven.md)
- [PRD Section 8 - Migration from lib/raven-lib.sh](docs/prd/PRD-Raven.md)
- [Go context cancellation patterns](https://go.dev/blog/context)
