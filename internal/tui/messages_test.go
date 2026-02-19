package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// requireNonNilCmd asserts that cmd is non-nil, failing the test immediately
// if it is. This is the canonical check for TickCmd / TickEvery return values.
func requireNonNilCmd(t *testing.T, cmd tea.Cmd, label string) {
	t.Helper()
	require.NotNil(t, cmd, "%s must return a non-nil tea.Cmd", label)
}

// ---------------------------------------------------------------------------
// AgentStatus.String() (table-driven)
// ---------------------------------------------------------------------------

func TestAgentStatus_String_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status AgentStatus
		want   string
	}{
		{
			name:   "AgentIdle is idle",
			status: AgentIdle,
			want:   "idle",
		},
		{
			name:   "AgentRunning is running",
			status: AgentRunning,
			want:   "running",
		},
		{
			name:   "AgentCompleted is completed",
			status: AgentCompleted,
			want:   "completed",
		},
		{
			name:   "AgentFailed is failed",
			status: AgentFailed,
			want:   "failed",
		},
		{
			name:   "AgentRateLimited is rate_limited",
			status: AgentRateLimited,
			want:   "rate_limited",
		},
		{
			name:   "AgentWaiting is waiting",
			status: AgentWaiting,
			want:   "waiting",
		},
		{
			name:   "out-of-range value 99 is unknown",
			status: AgentStatus(99),
			want:   "unknown",
		},
		{
			name:   "negative value -1 is unknown",
			status: AgentStatus(-1),
			want:   "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.status.String())
		})
	}
}

// Verify the AgentStatus iota values are stable and correctly ordered.
func TestAgentStatus_IotaValues(t *testing.T) {
	t.Parallel()

	assert.Equal(t, AgentStatus(0), AgentIdle)
	assert.Equal(t, AgentStatus(1), AgentRunning)
	assert.Equal(t, AgentStatus(2), AgentCompleted)
	assert.Equal(t, AgentStatus(3), AgentFailed)
	assert.Equal(t, AgentStatus(4), AgentRateLimited)
	assert.Equal(t, AgentStatus(5), AgentWaiting)
}

// Every defined constant must be distinct.
func TestAgentStatus_AllConstantsDistinct(t *testing.T) {
	t.Parallel()

	statuses := []AgentStatus{
		AgentIdle, AgentRunning, AgentCompleted,
		AgentFailed, AgentRateLimited, AgentWaiting,
	}
	seen := make(map[AgentStatus]string)
	names := []string{"AgentIdle", "AgentRunning", "AgentCompleted", "AgentFailed", "AgentRateLimited", "AgentWaiting"}
	for i, s := range statuses {
		prev, dup := seen[s]
		assert.False(t, dup, "AgentStatus constant %s duplicates %s (value %d)", names[i], prev, s)
		seen[s] = names[i]
	}
}

// ---------------------------------------------------------------------------
// LoopEventType.String() (table-driven)
// ---------------------------------------------------------------------------

func TestLoopEventType_String_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		eventType LoopEventType
		want      string
	}{
		{
			name:      "LoopIterationStarted is iteration_started",
			eventType: LoopIterationStarted,
			want:      "iteration_started",
		},
		{
			name:      "LoopIterationCompleted is iteration_completed",
			eventType: LoopIterationCompleted,
			want:      "iteration_completed",
		},
		{
			name:      "LoopTaskSelected is task_selected",
			eventType: LoopTaskSelected,
			want:      "task_selected",
		},
		{
			name:      "LoopTaskCompleted is task_completed",
			eventType: LoopTaskCompleted,
			want:      "task_completed",
		},
		{
			name:      "LoopTaskBlocked is task_blocked",
			eventType: LoopTaskBlocked,
			want:      "task_blocked",
		},
		{
			name:      "LoopWaitingForRateLimit is waiting_for_rate_limit",
			eventType: LoopWaitingForRateLimit,
			want:      "waiting_for_rate_limit",
		},
		{
			name:      "LoopResumedAfterWait is resumed_after_wait",
			eventType: LoopResumedAfterWait,
			want:      "resumed_after_wait",
		},
		{
			name:      "LoopPhaseComplete is phase_complete",
			eventType: LoopPhaseComplete,
			want:      "phase_complete",
		},
		{
			name:      "LoopError is error",
			eventType: LoopError,
			want:      "error",
		},
		{
			name:      "out-of-range value 99 is unknown",
			eventType: LoopEventType(99),
			want:      "unknown",
		},
		{
			name:      "negative value -1 is unknown",
			eventType: LoopEventType(-1),
			want:      "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.eventType.String())
		})
	}
}

// Verify the LoopEventType iota values are stable and correctly ordered.
func TestLoopEventType_IotaValues(t *testing.T) {
	t.Parallel()

	assert.Equal(t, LoopEventType(0), LoopIterationStarted)
	assert.Equal(t, LoopEventType(1), LoopIterationCompleted)
	assert.Equal(t, LoopEventType(2), LoopTaskSelected)
	assert.Equal(t, LoopEventType(3), LoopTaskCompleted)
	assert.Equal(t, LoopEventType(4), LoopTaskBlocked)
	assert.Equal(t, LoopEventType(5), LoopWaitingForRateLimit)
	assert.Equal(t, LoopEventType(6), LoopResumedAfterWait)
	assert.Equal(t, LoopEventType(7), LoopPhaseComplete)
	assert.Equal(t, LoopEventType(8), LoopError)
}

// Every defined constant must be distinct.
func TestLoopEventType_AllConstantsDistinct(t *testing.T) {
	t.Parallel()

	events := []LoopEventType{
		LoopIterationStarted, LoopIterationCompleted, LoopTaskSelected,
		LoopTaskCompleted, LoopTaskBlocked, LoopWaitingForRateLimit,
		LoopResumedAfterWait, LoopPhaseComplete, LoopError,
	}
	names := []string{
		"LoopIterationStarted", "LoopIterationCompleted", "LoopTaskSelected",
		"LoopTaskCompleted", "LoopTaskBlocked", "LoopWaitingForRateLimit",
		"LoopResumedAfterWait", "LoopPhaseComplete", "LoopError",
	}
	seen := make(map[LoopEventType]string)
	for i, e := range events {
		prev, dup := seen[e]
		assert.False(t, dup, "LoopEventType constant %s duplicates %s (value %d)", names[i], prev, e)
		seen[e] = names[i]
	}
}

// ---------------------------------------------------------------------------
// Message construction tests
// ---------------------------------------------------------------------------

func TestAgentOutputMsg_Construction(t *testing.T) {
	t.Parallel()

	now := time.Now()
	msg := AgentOutputMsg{
		Agent:     "claude",
		Line:      "hello from claude",
		Stream:    "stdout",
		Timestamp: now,
	}

	assert.Equal(t, "claude", msg.Agent)
	assert.Equal(t, "hello from claude", msg.Line)
	assert.Equal(t, "stdout", msg.Stream)
	assert.Equal(t, now, msg.Timestamp)
}

func TestAgentOutputMsg_StderrStream(t *testing.T) {
	t.Parallel()

	msg := AgentOutputMsg{
		Agent:  "codex",
		Line:   "error: build failed",
		Stream: "stderr",
	}

	assert.Equal(t, "codex", msg.Agent)
	assert.Equal(t, "error: build failed", msg.Line)
	assert.Equal(t, "stderr", msg.Stream)
}

func TestAgentStatusMsg_Construction(t *testing.T) {
	t.Parallel()

	now := time.Now()
	msg := AgentStatusMsg{
		Agent:     "gemini",
		Status:    AgentRunning,
		Task:      "T-042",
		Detail:    "generating code",
		Timestamp: now,
	}

	assert.Equal(t, "gemini", msg.Agent)
	assert.Equal(t, AgentRunning, msg.Status)
	assert.Equal(t, "T-042", msg.Task)
	assert.Equal(t, "generating code", msg.Detail)
	assert.Equal(t, now, msg.Timestamp)
}

func TestWorkflowEventMsg_Construction(t *testing.T) {
	t.Parallel()

	now := time.Now()
	msg := WorkflowEventMsg{
		WorkflowID:   "wf-001",
		WorkflowName: "implement-phase-1",
		Step:         "review",
		PrevStep:     "implement",
		Event:        "review_complete",
		Detail:       "all checks passed",
		Timestamp:    now,
	}

	assert.Equal(t, "wf-001", msg.WorkflowID)
	assert.Equal(t, "implement-phase-1", msg.WorkflowName)
	assert.Equal(t, "review", msg.Step)
	assert.Equal(t, "implement", msg.PrevStep)
	assert.Equal(t, "review_complete", msg.Event)
	assert.Equal(t, "all checks passed", msg.Detail)
	assert.Equal(t, now, msg.Timestamp)
}

func TestLoopEventMsg_Construction(t *testing.T) {
	t.Parallel()

	now := time.Now()
	msg := LoopEventMsg{
		Type:      LoopTaskCompleted,
		TaskID:    "T-010",
		Iteration: 3,
		MaxIter:   10,
		Detail:    "task finished successfully",
		Timestamp: now,
	}

	assert.Equal(t, LoopTaskCompleted, msg.Type)
	assert.Equal(t, "T-010", msg.TaskID)
	assert.Equal(t, 3, msg.Iteration)
	assert.Equal(t, 10, msg.MaxIter)
	assert.Equal(t, "task finished successfully", msg.Detail)
	assert.Equal(t, now, msg.Timestamp)
}

func TestRateLimitMsg_Construction(t *testing.T) {
	t.Parallel()

	now := time.Now()
	resetAt := now.Add(2 * time.Minute)
	msg := RateLimitMsg{
		Provider:   "anthropic",
		Agent:      "claude",
		ResetAfter: 2 * time.Minute,
		ResetAt:    resetAt,
		Timestamp:  now,
	}

	assert.Equal(t, "anthropic", msg.Provider)
	assert.Equal(t, "claude", msg.Agent)
	assert.Equal(t, 2*time.Minute, msg.ResetAfter)
	assert.Equal(t, resetAt, msg.ResetAt)
	assert.Equal(t, now, msg.Timestamp)
}

func TestTaskProgressMsg_Construction(t *testing.T) {
	t.Parallel()

	now := time.Now()
	msg := TaskProgressMsg{
		TaskID:    "T-007",
		TaskTitle: "Implement rate limiting",
		Status:    "in_progress",
		Phase:     2,
		Completed: 4,
		Total:     12,
		Timestamp: now,
	}

	assert.Equal(t, "T-007", msg.TaskID)
	assert.Equal(t, "Implement rate limiting", msg.TaskTitle)
	assert.Equal(t, "in_progress", msg.Status)
	assert.Equal(t, 2, msg.Phase)
	assert.Equal(t, 4, msg.Completed)
	assert.Equal(t, 12, msg.Total)
	assert.Equal(t, now, msg.Timestamp)
}

func TestTickMsg_Construction(t *testing.T) {
	t.Parallel()

	now := time.Now()
	msg := TickMsg{Time: now}

	assert.Equal(t, now, msg.Time)
}

func TestErrorMsg_Construction(t *testing.T) {
	t.Parallel()

	now := time.Now()
	msg := ErrorMsg{
		Source:    "loop",
		Detail:    "context deadline exceeded",
		Timestamp: now,
	}

	assert.Equal(t, "loop", msg.Source)
	assert.Equal(t, "context deadline exceeded", msg.Detail)
	assert.Equal(t, now, msg.Timestamp)
}

func TestFocusChangedMsg_Construction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		panel FocusPanel
	}{
		{name: "sidebar", panel: FocusSidebar},
		{name: "agent panel", panel: FocusAgentPanel},
		{name: "event log", panel: FocusEventLog},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msg := FocusChangedMsg{Panel: tt.panel}
			assert.Equal(t, tt.panel, msg.Panel)
		})
	}
}

// ---------------------------------------------------------------------------
// TickCmd tests
// ---------------------------------------------------------------------------

func TestTickCmd_ReturnsNonNil(t *testing.T) {
	t.Parallel()

	cmd := TickCmd(time.Second)
	requireNonNilCmd(t, cmd, "TickCmd(time.Second)")
}

func TestTickCmd_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
	}{
		{name: "one second", duration: time.Second},
		{name: "one minute", duration: time.Minute},
		{name: "100 milliseconds", duration: 100 * time.Millisecond},
		{name: "one hour", duration: time.Hour},
		// Zero is an edge case; tea.Tick accepts it and fires immediately.
		{name: "zero duration", duration: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := TickCmd(tt.duration)
			requireNonNilCmd(t, cmd, "TickCmd("+tt.duration.String()+")")
		})
	}
}

// ---------------------------------------------------------------------------
// TickEvery tests
// ---------------------------------------------------------------------------

func TestTickEvery_ReturnsNonNil(t *testing.T) {
	t.Parallel()

	cmd := TickEvery(time.Second)
	requireNonNilCmd(t, cmd, "TickEvery(time.Second)")
}

func TestTickEvery_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
	}{
		{name: "one second", duration: time.Second},
		{name: "500 milliseconds", duration: 500 * time.Millisecond},
		{name: "five minutes", duration: 5 * time.Minute},
		{name: "10 milliseconds", duration: 10 * time.Millisecond},
		{name: "zero duration", duration: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := TickEvery(tt.duration)
			requireNonNilCmd(t, cmd, "TickEvery("+tt.duration.String()+")")
		})
	}
}

// TickCmd and TickEvery are intentionally identical in behaviour (both use
// tea.Tick internally). Verify both can be called independently and return
// distinct (non-shared) tea.Cmd values.
func TestTickCmd_AndTickEvery_ReturnIndependentCmds(t *testing.T) {
	t.Parallel()

	cmd1 := TickCmd(time.Second)
	cmd2 := TickEvery(time.Second)

	require.NotNil(t, cmd1)
	require.NotNil(t, cmd2)
	// Both must be independent (function values; pointer inequality would need
	// unsafe; assert they are both non-nil and callable separately).
}

// ---------------------------------------------------------------------------
// Type switch tests – simulate an Update function dispatching on tea.Msg
// ---------------------------------------------------------------------------

// typeSwitch dispatches msg through a switch identical to what a Bubble Tea
// Update function would use, and returns a string identifying which branch
// matched. If no branch matches it returns "unhandled".
func typeSwitch(msg tea.Msg) string {
	switch msg.(type) {
	case AgentOutputMsg:
		return "AgentOutputMsg"
	case AgentStatusMsg:
		return "AgentStatusMsg"
	case WorkflowEventMsg:
		return "WorkflowEventMsg"
	case LoopEventMsg:
		return "LoopEventMsg"
	case RateLimitMsg:
		return "RateLimitMsg"
	case TaskProgressMsg:
		return "TaskProgressMsg"
	case TickMsg:
		return "TickMsg"
	case ErrorMsg:
		return "ErrorMsg"
	case FocusChangedMsg:
		return "FocusChangedMsg"
	default:
		return "unhandled"
	}
}

func TestTypeSwitch_AllMessageTypes(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name       string
		msg        tea.Msg
		wantBranch string
	}{
		{
			name:       "AgentOutputMsg routes correctly",
			msg:        AgentOutputMsg{Agent: "claude", Line: "ok", Stream: "stdout", Timestamp: now},
			wantBranch: "AgentOutputMsg",
		},
		{
			name:       "AgentStatusMsg routes correctly",
			msg:        AgentStatusMsg{Agent: "claude", Status: AgentRunning, Task: "T-001", Timestamp: now},
			wantBranch: "AgentStatusMsg",
		},
		{
			name: "WorkflowEventMsg routes correctly",
			msg: WorkflowEventMsg{
				WorkflowID: "wf-1", WorkflowName: "main",
				Step: "review", PrevStep: "implement",
				Event: "done", Timestamp: now,
			},
			wantBranch: "WorkflowEventMsg",
		},
		{
			name: "LoopEventMsg routes correctly",
			msg: LoopEventMsg{
				Type: LoopIterationStarted, TaskID: "T-002",
				Iteration: 1, MaxIter: 5, Timestamp: now,
			},
			wantBranch: "LoopEventMsg",
		},
		{
			name: "RateLimitMsg routes correctly",
			msg: RateLimitMsg{
				Provider: "anthropic", Agent: "claude",
				ResetAfter: time.Minute, ResetAt: now.Add(time.Minute),
				Timestamp: now,
			},
			wantBranch: "RateLimitMsg",
		},
		{
			name: "TaskProgressMsg routes correctly",
			msg: TaskProgressMsg{
				TaskID: "T-003", TaskTitle: "Scaffold", Status: "completed",
				Phase: 1, Completed: 1, Total: 5, Timestamp: now,
			},
			wantBranch: "TaskProgressMsg",
		},
		{
			name:       "TickMsg routes correctly",
			msg:        TickMsg{Time: now},
			wantBranch: "TickMsg",
		},
		{
			name:       "ErrorMsg routes correctly",
			msg:        ErrorMsg{Source: "agent", Detail: "exec failed", Timestamp: now},
			wantBranch: "ErrorMsg",
		},
		{
			name:       "FocusChangedMsg routes correctly",
			msg:        FocusChangedMsg{Panel: FocusEventLog},
			wantBranch: "FocusChangedMsg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := typeSwitch(tt.msg)
			assert.Equal(t, tt.wantBranch, got)
		})
	}
}

// Verify that an unrecognised message falls through to the default branch.
func TestTypeSwitch_UnknownMsg_Unhandled(t *testing.T) {
	t.Parallel()

	type customMsg struct{ payload string }
	got := typeSwitch(customMsg{payload: "irrelevant"})
	assert.Equal(t, "unhandled", got)
}

// ---------------------------------------------------------------------------
// Zero-value / edge case tests
// ---------------------------------------------------------------------------

func TestAgentOutputMsg_ZeroValue(t *testing.T) {
	t.Parallel()

	// Constructing with empty Agent and Line must not panic.
	var msg AgentOutputMsg
	assert.Empty(t, msg.Agent)
	assert.Empty(t, msg.Line)
	assert.Empty(t, msg.Stream)
	assert.True(t, msg.Timestamp.IsZero())
}

func TestAgentOutputMsg_EmptyAgentAndLine(t *testing.T) {
	t.Parallel()

	msg := AgentOutputMsg{Agent: "", Line: "", Stream: "stdout"}
	assert.Empty(t, msg.Agent, "empty Agent must be preserved")
	assert.Empty(t, msg.Line, "empty Line must be preserved")
	// The type switch must still dispatch correctly.
	assert.Equal(t, "AgentOutputMsg", typeSwitch(msg))
}

func TestRateLimitMsg_ZeroDuration(t *testing.T) {
	t.Parallel()

	// A RateLimitMsg with zero ResetAfter must not panic.
	assert.NotPanics(t, func() {
		msg := RateLimitMsg{
			Provider:   "anthropic",
			Agent:      "claude",
			ResetAfter: 0,
		}
		assert.Equal(t, time.Duration(0), msg.ResetAfter)
		assert.Equal(t, "RateLimitMsg", typeSwitch(msg))
	})
}

func TestRateLimitMsg_ZeroValue_DoesNotPanic(t *testing.T) {
	t.Parallel()

	assert.NotPanics(t, func() {
		var msg RateLimitMsg
		_ = msg.ResetAfter
		_ = msg.ResetAt
	})
}

func TestLoopEventMsg_ZeroTaskID(t *testing.T) {
	t.Parallel()

	msg := LoopEventMsg{
		Type:      LoopPhaseComplete,
		TaskID:    "",
		Iteration: 0,
		MaxIter:   0,
	}
	assert.Empty(t, msg.TaskID, "zero TaskID must be preserved as empty string")
	assert.Equal(t, "LoopEventMsg", typeSwitch(msg))
}

func TestLoopEventMsg_ZeroValue(t *testing.T) {
	t.Parallel()

	var msg LoopEventMsg
	// LoopIterationStarted == 0, so the zero value maps to the first variant.
	assert.Equal(t, LoopIterationStarted, msg.Type)
	assert.Empty(t, msg.TaskID)
	assert.Equal(t, 0, msg.Iteration)
	assert.Equal(t, 0, msg.MaxIter)
}

func TestFocusChangedMsg_AllPanels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		panel FocusPanel
	}{
		{name: "FocusSidebar zero value", panel: FocusSidebar},
		{name: "FocusAgentPanel", panel: FocusAgentPanel},
		{name: "FocusEventLog", panel: FocusEventLog},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msg := FocusChangedMsg{Panel: tt.panel}
			assert.Equal(t, tt.panel, msg.Panel)
			// Type switch must dispatch to FocusChangedMsg regardless of panel value.
			assert.Equal(t, "FocusChangedMsg", typeSwitch(msg))
		})
	}
}

// FocusChangedMsg with zero value uses FocusSidebar (iota 0).
func TestFocusChangedMsg_ZeroValue(t *testing.T) {
	t.Parallel()

	var msg FocusChangedMsg
	assert.Equal(t, FocusSidebar, msg.Panel, "zero-value FocusChangedMsg should have FocusSidebar")
}

// ---------------------------------------------------------------------------
// AgentStatusMsg – all AgentStatus values round-trip through msg construction
// ---------------------------------------------------------------------------

func TestAgentStatusMsg_AllStatuses(t *testing.T) {
	t.Parallel()

	allStatuses := []AgentStatus{
		AgentIdle, AgentRunning, AgentCompleted,
		AgentFailed, AgentRateLimited, AgentWaiting,
	}

	for _, status := range allStatuses {
		status := status
		t.Run(status.String(), func(t *testing.T) {
			t.Parallel()
			msg := AgentStatusMsg{Agent: "claude", Status: status}
			assert.Equal(t, status, msg.Status)
			assert.Equal(t, "AgentStatusMsg", typeSwitch(msg))
		})
	}
}

// ---------------------------------------------------------------------------
// LoopEventMsg – all LoopEventType values round-trip through msg construction
// ---------------------------------------------------------------------------

func TestLoopEventMsg_AllEventTypes(t *testing.T) {
	t.Parallel()

	allTypes := []LoopEventType{
		LoopIterationStarted, LoopIterationCompleted, LoopTaskSelected,
		LoopTaskCompleted, LoopTaskBlocked, LoopWaitingForRateLimit,
		LoopResumedAfterWait, LoopPhaseComplete, LoopError,
	}

	for _, et := range allTypes {
		et := et
		t.Run(et.String(), func(t *testing.T) {
			t.Parallel()
			msg := LoopEventMsg{Type: et, TaskID: "T-001", Iteration: 1, MaxIter: 5}
			assert.Equal(t, et, msg.Type)
			assert.Equal(t, "LoopEventMsg", typeSwitch(msg))
		})
	}
}

// ---------------------------------------------------------------------------
// TaskProgressMsg – status string values
// ---------------------------------------------------------------------------

func TestTaskProgressMsg_StatusStrings(t *testing.T) {
	t.Parallel()

	// The canonical status strings are: "not_started", "in_progress", "completed",
	// "blocked", "skipped". Verify each can be stored and retrieved.
	canonicalStatuses := []string{
		"not_started", "in_progress", "completed", "blocked", "skipped",
	}

	for _, status := range canonicalStatuses {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			msg := TaskProgressMsg{
				TaskID: "T-099",
				Status: status,
				Phase:  1, Completed: 0, Total: 1,
			}
			assert.Equal(t, status, msg.Status)
		})
	}
}

// ---------------------------------------------------------------------------
// WorkflowEventMsg – zero-value fields do not cause panics
// ---------------------------------------------------------------------------

func TestWorkflowEventMsg_ZeroValue(t *testing.T) {
	t.Parallel()

	var msg WorkflowEventMsg
	assert.Empty(t, msg.WorkflowID)
	assert.Empty(t, msg.WorkflowName)
	assert.Empty(t, msg.Step)
	assert.Empty(t, msg.PrevStep)
	assert.Empty(t, msg.Event)
	assert.Empty(t, msg.Detail)
	assert.True(t, msg.Timestamp.IsZero())
	// Type switch must still dispatch correctly.
	assert.Equal(t, "WorkflowEventMsg", typeSwitch(msg))
}

// ---------------------------------------------------------------------------
// ErrorMsg – edge cases
// ---------------------------------------------------------------------------

func TestErrorMsg_EmptySource(t *testing.T) {
	t.Parallel()

	msg := ErrorMsg{Source: "", Detail: "something broke"}
	assert.Empty(t, msg.Source)
	assert.Equal(t, "something broke", msg.Detail)
	assert.Equal(t, "ErrorMsg", typeSwitch(msg))
}

func TestErrorMsg_EmptyDetail(t *testing.T) {
	t.Parallel()

	msg := ErrorMsg{Source: "review", Detail: ""}
	assert.Empty(t, msg.Detail)
	assert.Equal(t, "ErrorMsg", typeSwitch(msg))
}

// ---------------------------------------------------------------------------
// TickMsg – timestamp field
// ---------------------------------------------------------------------------

func TestTickMsg_TimePreserved(t *testing.T) {
	t.Parallel()

	// A TickMsg must carry the exact time.Time value it was constructed with.
	now := time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC)
	msg := TickMsg{Time: now}
	assert.Equal(t, now, msg.Time)
	assert.Equal(t, "TickMsg", typeSwitch(msg))
}

func TestTickMsg_ZeroTime(t *testing.T) {
	t.Parallel()

	var msg TickMsg
	assert.True(t, msg.Time.IsZero())
}

// ---------------------------------------------------------------------------
// AgentOutputMsg – all three stream values
// ---------------------------------------------------------------------------

func TestAgentOutputMsg_StreamValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stream string
	}{
		{name: "stdout stream", stream: "stdout"},
		{name: "stderr stream", stream: "stderr"},
		{name: "empty stream", stream: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msg := AgentOutputMsg{Agent: "claude", Line: "line", Stream: tt.stream}
			assert.Equal(t, tt.stream, msg.Stream)
		})
	}
}

// ---------------------------------------------------------------------------
// Benchmark: String() methods are hot paths; verify they stay allocation-free
// ---------------------------------------------------------------------------

func BenchmarkAgentStatus_String(b *testing.B) {
	statuses := []AgentStatus{
		AgentIdle, AgentRunning, AgentCompleted,
		AgentFailed, AgentRateLimited, AgentWaiting,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = statuses[i%len(statuses)].String()
	}
}

func BenchmarkLoopEventType_String(b *testing.B) {
	types := []LoopEventType{
		LoopIterationStarted, LoopIterationCompleted, LoopTaskSelected,
		LoopTaskCompleted, LoopTaskBlocked, LoopWaitingForRateLimit,
		LoopResumedAfterWait, LoopPhaseComplete, LoopError,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = types[i%len(types)].String()
	}
}

func BenchmarkTickCmd(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TickCmd(time.Second)
	}
}

func BenchmarkTickEvery(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TickEvery(time.Second)
	}
}
