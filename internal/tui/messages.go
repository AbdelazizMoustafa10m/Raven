package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Agent Messages
// ---------------------------------------------------------------------------

// AgentOutputMsg represents a single line of output from an agent process.
// Stream is either "stdout" or "stderr".
type AgentOutputMsg struct {
	// Agent is the name of the agent that produced this output (e.g. "claude").
	Agent string
	// Line is the raw text line received from the agent process.
	Line string
	// Stream indicates whether the line came from stdout or stderr.
	Stream string
	// Timestamp records when this line was received.
	Timestamp time.Time
}

// AgentStatus represents the current lifecycle state of an agent.
type AgentStatus int

const (
	// AgentIdle means the agent is available but not currently processing work.
	AgentIdle AgentStatus = iota
	// AgentRunning means the agent is actively executing a task.
	AgentRunning
	// AgentCompleted means the agent finished its task successfully.
	AgentCompleted
	// AgentFailed means the agent encountered a terminal error.
	AgentFailed
	// AgentRateLimited means the agent is paused due to provider rate limits.
	AgentRateLimited
	// AgentWaiting means the agent is waiting for a dependency or resource.
	AgentWaiting
)

// agentStatusStrings maps each AgentStatus constant to its human-readable label.
var agentStatusStrings = []string{
	"idle",
	"running",
	"completed",
	"failed",
	"rate_limited",
	"waiting",
}

// String returns a human-readable label for the AgentStatus.
// Returns "unknown" for values outside the defined range.
func (s AgentStatus) String() string {
	if int(s) < 0 || int(s) >= len(agentStatusStrings) {
		return "unknown"
	}
	return agentStatusStrings[s]
}

// AgentStatusMsg signals an agent lifecycle change.
// It is dispatched whenever an agent transitions between states (e.g. from
// AgentIdle to AgentRunning when a task begins).
type AgentStatusMsg struct {
	// Agent is the name of the agent whose status changed (e.g. "claude").
	Agent string
	// Status is the new lifecycle state of the agent.
	Status AgentStatus
	// Task is the identifier of the task being processed, if applicable.
	Task string
	// Detail is an optional human-readable description of the transition.
	Detail string
	// Timestamp records when the status transition occurred.
	Timestamp time.Time
}

// ---------------------------------------------------------------------------
// Workflow Messages
// ---------------------------------------------------------------------------

// WorkflowEventMsg signals a workflow engine state change.
// It carries enough context for the TUI to render meaningful transitions in
// the event log and status bar.
type WorkflowEventMsg struct {
	// WorkflowID is the unique identifier of the workflow instance.
	WorkflowID string
	// WorkflowName is the human-readable name of the workflow.
	WorkflowName string
	// Step is the current step the workflow has transitioned into.
	Step string
	// PrevStep is the step the workflow was in before this transition.
	PrevStep string
	// Event is the triggering event name that caused the transition.
	Event string
	// Detail is an optional human-readable description or metadata string.
	Detail string
	// Timestamp records when the workflow event was emitted.
	Timestamp time.Time
}

// ---------------------------------------------------------------------------
// Loop Messages
// ---------------------------------------------------------------------------

// LoopEventType categorizes implementation loop events for display and routing.
type LoopEventType int

const (
	// LoopIterationStarted fires at the beginning of each loop iteration.
	LoopIterationStarted LoopEventType = iota
	// LoopIterationCompleted fires when an iteration finishes without error.
	LoopIterationCompleted
	// LoopTaskSelected fires when the selector picks the next task to run.
	LoopTaskSelected
	// LoopTaskCompleted fires when a task finishes successfully.
	LoopTaskCompleted
	// LoopTaskBlocked fires when a task cannot proceed due to unmet dependencies.
	LoopTaskBlocked
	// LoopWaitingForRateLimit fires when the loop pauses due to provider rate limits.
	LoopWaitingForRateLimit
	// LoopResumedAfterWait fires when the loop resumes after a rate-limit wait.
	LoopResumedAfterWait
	// LoopPhaseComplete fires when all tasks in the current phase are done.
	LoopPhaseComplete
	// LoopError fires when the loop encounters a non-fatal error and continues.
	LoopError
)

// loopEventTypeStrings maps each LoopEventType constant to its human-readable label.
var loopEventTypeStrings = []string{
	"iteration_started",
	"iteration_completed",
	"task_selected",
	"task_completed",
	"task_blocked",
	"waiting_for_rate_limit",
	"resumed_after_wait",
	"phase_complete",
	"error",
}

// String returns a human-readable label for the LoopEventType.
// Returns "unknown" for values outside the defined range.
func (t LoopEventType) String() string {
	if int(t) < 0 || int(t) >= len(loopEventTypeStrings) {
		return "unknown"
	}
	return loopEventTypeStrings[t]
}

// LoopEventMsg signals an implementation loop event.
// It carries iteration counters and task context so the TUI can display
// accurate progress and rate-limit countdown information.
type LoopEventMsg struct {
	// Type categorizes the kind of loop event.
	Type LoopEventType
	// TaskID is the identifier of the task associated with this event, if any.
	TaskID string
	// Iteration is the current loop iteration number (1-based).
	Iteration int
	// MaxIter is the configured maximum number of iterations.
	MaxIter int
	// Detail is an optional human-readable description or error message.
	Detail string
	// Timestamp records when this loop event was emitted.
	Timestamp time.Time
}

// ---------------------------------------------------------------------------
// Rate Limit Messages
// ---------------------------------------------------------------------------

// RateLimitMsg signals a rate-limit event with countdown information.
// The TUI uses ResetAfter / ResetAt to display a live countdown timer until
// the provider allows new requests.
type RateLimitMsg struct {
	// Provider is the AI provider that issued the rate limit (e.g. "anthropic").
	Provider string
	// Agent is the agent name that hit the rate limit (e.g. "claude").
	Agent string
	// ResetAfter is the duration to wait before the rate limit clears.
	ResetAfter time.Duration
	// ResetAt is the absolute time at which the rate limit is expected to clear.
	ResetAt time.Time
	// Timestamp records when the rate-limit event was detected.
	Timestamp time.Time
}

// ---------------------------------------------------------------------------
// Task Progress Messages
// ---------------------------------------------------------------------------

// TaskProgressMsg signals a task state change for progress tracking.
// Status values mirror the canonical task status strings used throughout Raven:
// "not_started", "in_progress", "completed", "blocked", or "skipped".
type TaskProgressMsg struct {
	// TaskID is the unique identifier of the task (e.g. "T-001").
	TaskID string
	// TaskTitle is the human-readable title of the task.
	TaskTitle string
	// Status is the new task status string.
	Status string
	// Phase is the phase number that owns this task (1-based).
	Phase int
	// Completed is the number of tasks completed so far in this phase.
	Completed int
	// Total is the total number of tasks in this phase.
	Total int
	// Timestamp records when this progress update was emitted.
	Timestamp time.Time
}

// ---------------------------------------------------------------------------
// Internal TUI Messages
// ---------------------------------------------------------------------------

// TickMsg is sent periodically to trigger timer updates such as rate-limit
// countdowns and elapsed-time displays.
type TickMsg struct {
	// Time is the wall-clock time at which the tick fired.
	Time time.Time
}

// ErrorMsg represents a non-fatal error to display in the event log.
// Fatal errors should cause program termination via tea.Quit; ErrorMsg is
// reserved for recoverable issues that the user should be aware of.
type ErrorMsg struct {
	// Source identifies the component that generated the error (e.g. "loop", "agent").
	Source string
	// Detail is the human-readable error description.
	Detail string
	// Timestamp records when the error was observed.
	Timestamp time.Time
}

// FocusChangedMsg signals that keyboard focus moved to a different panel.
// The TUI dispatches this message whenever the user navigates between the
// sidebar, agent panel, and event log.
type FocusChangedMsg struct {
	// Panel is the panel that has received focus.
	// FocusPanel is defined in app.go (same package).
	Panel FocusPanel
}

// ---------------------------------------------------------------------------
// Helper Functions
// ---------------------------------------------------------------------------

// TickCmd returns a tea.Cmd that sends a single TickMsg after duration d.
// Use this helper instead of time.After in goroutines to stay within Bubble
// Tea's Elm architecture and avoid data races.
func TickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return TickMsg{Time: t}
	})
}

// TickEvery returns a tea.Cmd that sends a TickMsg after duration d.
// The caller's Update handler should call TickEvery again upon receiving a
// TickMsg to create recurring ticks via the recursive scheduling pattern:
//
//	case TickMsg:
//	    // update state...
//	    return m, TickEvery(interval)
func TickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return TickMsg{Time: t}
	})
}
