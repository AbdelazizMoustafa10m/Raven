package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// makeEventLog is a convenience constructor that creates an EventLogModel with
// sensible defaults for use in tests.
func makeEventLog(t *testing.T, width, height int) EventLogModel {
	t.Helper()
	el := NewEventLogModel(DefaultTheme())
	el.SetDimensions(width, height)
	return el
}

// sendEventLogMsg dispatches a tea.Msg to the EventLogModel and returns the
// updated model. The returned command is intentionally discarded for callers
// that do not need to inspect it.
func sendEventLogMsg(el EventLogModel, msg tea.Msg) EventLogModel {
	updated, _ := el.Update(msg)
	return updated
}

// pressEventLogKey dispatches a rune key tea.KeyMsg to the EventLogModel and
// returns the updated model and command.
func pressEventLogKey(el EventLogModel, r rune) (EventLogModel, tea.Cmd) {
	return el.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
}

// pressEventLogSpecialKey dispatches a special key (non-rune) to the
// EventLogModel and returns the updated model and command.
func pressEventLogSpecialKey(el EventLogModel, kt tea.KeyType) (EventLogModel, tea.Cmd) {
	return el.Update(tea.KeyMsg{Type: kt})
}

// ---------------------------------------------------------------------------
// TestNewEventLogModel_Defaults
// ---------------------------------------------------------------------------

// TestNewEventLogModel_Defaults verifies that a freshly constructed model has
// visible=true, autoScroll=true, empty entries, and zero dimensions.
func TestNewEventLogModel_Defaults(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())

	assert.True(t, el.visible, "visible must be true after construction")
	assert.True(t, el.autoScroll, "autoScroll must be true after construction")
	assert.Empty(t, el.entries, "entries must be empty after construction")
	assert.Equal(t, 0, el.width, "width must be 0 after construction")
	assert.Equal(t, 0, el.height, "height must be 0 after construction")
	assert.False(t, el.focused, "focused must be false after construction")
}

// ---------------------------------------------------------------------------
// TestAddEntry_*
// ---------------------------------------------------------------------------

// TestAddEntry_AppendsEntry verifies that adding one entry results in exactly
// one entry stored with the correct category and message.
func TestAddEntry_AppendsEntry(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	el.AddEntry(EventInfo, "hello world")

	require.Len(t, el.entries, 1, "entries must contain exactly one entry")
	assert.Equal(t, EventInfo, el.entries[0].Category, "category must be EventInfo")
	assert.Equal(t, "hello world", el.entries[0].Message, "message must match")
}

// TestAddEntry_EvictsOldestWhenOverLimit verifies that when entries exceed
// MaxEventLogEntries the buffer is trimmed to exactly MaxEventLogEntries,
// retaining only the most recent entries.
func TestAddEntry_EvictsOldestWhenOverLimit(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	total := MaxEventLogEntries + 100
	for i := 0; i < total; i++ {
		el.AddEntry(EventInfo, fmt.Sprintf("entry-%d", i))
	}

	require.Len(t, el.entries, MaxEventLogEntries,
		"entries must be capped at MaxEventLogEntries after overflow")

	// The oldest retained entry must be entry-100 (the first 100 were evicted).
	assert.Equal(t, fmt.Sprintf("entry-%d", 100), el.entries[0].Message,
		"oldest retained entry must be entry-100")

	// The newest entry must be the last one added.
	assert.Equal(t, fmt.Sprintf("entry-%d", total-1), el.entries[len(el.entries)-1].Message,
		"newest retained entry must be the last added entry")
}

// TestAddEntry_ExactlyAtLimit verifies that adding exactly MaxEventLogEntries
// entries causes no eviction — the buffer should hold all of them.
func TestAddEntry_ExactlyAtLimit(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	for i := 0; i < MaxEventLogEntries; i++ {
		el.AddEntry(EventInfo, fmt.Sprintf("entry-%d", i))
	}

	assert.Len(t, el.entries, MaxEventLogEntries,
		"entries must hold exactly MaxEventLogEntries when filled to capacity")
}

// ---------------------------------------------------------------------------
// TestSetVisible_*
// ---------------------------------------------------------------------------

// TestSetVisible_TogglesVisibility verifies that SetVisible correctly
// transitions the model between hidden and visible states, and that IsVisible
// reflects the current state.
func TestSetVisible_TogglesVisibility(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	require.True(t, el.IsVisible(), "model must start visible")

	el.SetVisible(false)
	assert.False(t, el.IsVisible(), "IsVisible must return false after SetVisible(false)")

	el.SetVisible(true)
	assert.True(t, el.IsVisible(), "IsVisible must return true after SetVisible(true)")
}

// ---------------------------------------------------------------------------
// TestView_*
// ---------------------------------------------------------------------------

// TestView_ReturnsEmptyWhenHidden verifies that View() returns an empty string
// when the panel is hidden, even if it has dimensions and entries.
func TestView_ReturnsEmptyWhenHidden(t *testing.T) {
	t.Parallel()

	el := makeEventLog(t, 80, 20)
	el.AddEntry(EventInfo, "should not appear")
	el.SetVisible(false)

	assert.Equal(t, "", el.View(), "View must return empty string when panel is hidden")
}

// TestView_ReturnsEmptyWhenNoDimensions verifies that View() returns an empty
// string when SetDimensions has never been called.
func TestView_ReturnsEmptyWhenNoDimensions(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	el.AddEntry(EventInfo, "has an entry")
	// Deliberately do NOT call SetDimensions.

	assert.Equal(t, "", el.View(), "View must return empty string when dimensions are zero")
}

// TestView_ShowsNoEventsPlaceholder verifies that when dimensions are set but
// no entries exist, View() renders the "No events yet" placeholder text.
func TestView_ShowsNoEventsPlaceholder(t *testing.T) {
	t.Parallel()

	el := makeEventLog(t, 80, 20)

	output := stripANSIPanel(el.View())
	assert.Contains(t, output, "No events yet",
		"View must show placeholder when entry list is empty")
}

// TestView_ContainsTimestampAndMessage verifies that after adding one entry
// the rendered output contains the message text.
func TestView_ContainsTimestampAndMessage(t *testing.T) {
	t.Parallel()

	el := makeEventLog(t, 80, 20)
	el.AddEntry(EventInfo, "test message alpha")

	output := stripANSIPanel(el.View())
	assert.Contains(t, output, "test message alpha",
		"View must contain the entry message text")
}

// ---------------------------------------------------------------------------
// TestUpdate_WorkflowEventMsg_*
// ---------------------------------------------------------------------------

// TestUpdate_WorkflowEventMsg_AddsEntry verifies that a WorkflowEventMsg
// results in an entry being appended to the log with message text that
// includes the workflow name, source step, and destination step.
func TestUpdate_WorkflowEventMsg_AddsEntry(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	el = sendEventLogMsg(el, WorkflowEventMsg{
		WorkflowID:   "wf-001",
		WorkflowName: "pipeline",
		Step:         "review",
		PrevStep:     "implement",
		Event:        "step_changed",
		Timestamp:    time.Now(),
	})

	require.Len(t, el.entries, 1, "one entry must be added for WorkflowEventMsg")
	msg := el.entries[0].Message
	assert.Contains(t, msg, "pipeline", "message must contain workflow name")
	assert.Contains(t, msg, "implement", "message must contain previous step")
	assert.Contains(t, msg, "review", "message must contain current step")
}

// TestUpdate_WorkflowEventMsg_FailureCategory verifies that a WorkflowEventMsg
// whose Event field contains "failed" is classified as EventError.
func TestUpdate_WorkflowEventMsg_FailureCategory(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	el = sendEventLogMsg(el, WorkflowEventMsg{
		WorkflowName: "pipeline",
		Event:        "failed",
		Timestamp:    time.Now(),
	})

	require.Len(t, el.entries, 1)
	assert.Equal(t, EventError, el.entries[0].Category,
		"event with 'failed' in Event field must be classified as EventError")
}

// ---------------------------------------------------------------------------
// TestUpdate_LoopEventMsg_*
// ---------------------------------------------------------------------------

// TestUpdate_LoopEventMsg_TaskSelected verifies that a LoopEventMsg with
// Type=LoopTaskSelected produces an entry containing the task ID and iteration.
func TestUpdate_LoopEventMsg_TaskSelected(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	el = sendEventLogMsg(el, LoopEventMsg{
		Type:      LoopTaskSelected,
		TaskID:    "T-007",
		Iteration: 3,
		Timestamp: time.Now(),
	})

	require.Len(t, el.entries, 1, "one entry must be added for LoopEventMsg")
	msg := el.entries[0].Message
	assert.Contains(t, msg, "T-007", "message must contain task ID")
	assert.Contains(t, msg, "3", "message must contain iteration number")
}

// TestUpdate_LoopEventMsg_AllTypes verifies that each LoopEventType value maps
// to the expected EventCategory.
func TestUpdate_LoopEventMsg_AllTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		loopType     LoopEventType
		wantCategory EventCategory
	}{
		{"LoopIterationStarted", LoopIterationStarted, EventInfo},
		{"LoopIterationCompleted", LoopIterationCompleted, EventSuccess},
		{"LoopTaskSelected", LoopTaskSelected, EventInfo},
		{"LoopTaskCompleted", LoopTaskCompleted, EventSuccess},
		{"LoopTaskBlocked", LoopTaskBlocked, EventWarning},
		{"LoopWaitingForRateLimit", LoopWaitingForRateLimit, EventWarning},
		{"LoopResumedAfterWait", LoopResumedAfterWait, EventInfo},
		{"LoopPhaseComplete", LoopPhaseComplete, EventSuccess},
		{"LoopError", LoopError, EventError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			el := NewEventLogModel(DefaultTheme())
			el = sendEventLogMsg(el, LoopEventMsg{
				Type:      tt.loopType,
				TaskID:    "T-001",
				Iteration: 1,
				Timestamp: time.Now(),
			})

			require.Len(t, el.entries, 1, "one entry must be added per LoopEventMsg")
			assert.Equal(t, tt.wantCategory, el.entries[0].Category,
				"LoopEventType %v must map to category %v", tt.loopType, tt.wantCategory)
		})
	}
}

// ---------------------------------------------------------------------------
// TestUpdate_AgentStatusMsg_*
// ---------------------------------------------------------------------------

// TestUpdate_AgentStatusMsg_Running verifies that an AgentStatusMsg with
// Status=AgentRunning produces an entry whose message contains both the agent
// name and the task ID.
func TestUpdate_AgentStatusMsg_Running(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	el = sendEventLogMsg(el, AgentStatusMsg{
		Agent:     "claude",
		Status:    AgentRunning,
		Task:      "T-007",
		Timestamp: time.Now(),
	})

	require.Len(t, el.entries, 1, "one entry must be added for AgentStatusMsg")
	msg := el.entries[0].Message
	assert.Contains(t, msg, "claude", "message must contain agent name")
	assert.Contains(t, msg, "T-007", "message must contain task ID")
}

// TestUpdate_AgentStatusMsg_AllStatuses verifies that each AgentStatus value
// maps to the expected EventCategory.
func TestUpdate_AgentStatusMsg_AllStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		status       AgentStatus
		wantCategory EventCategory
	}{
		{"AgentIdle", AgentIdle, EventInfo},
		{"AgentRunning", AgentRunning, EventInfo},
		{"AgentCompleted", AgentCompleted, EventSuccess},
		{"AgentFailed", AgentFailed, EventError},
		{"AgentRateLimited", AgentRateLimited, EventWarning},
		{"AgentWaiting", AgentWaiting, EventWarning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			el := NewEventLogModel(DefaultTheme())
			el = sendEventLogMsg(el, AgentStatusMsg{
				Agent:     "claude",
				Status:    tt.status,
				Timestamp: time.Now(),
			})

			require.Len(t, el.entries, 1, "one entry must be added per AgentStatusMsg")
			assert.Equal(t, tt.wantCategory, el.entries[0].Category,
				"AgentStatus %v must map to category %v", tt.status, tt.wantCategory)
		})
	}
}

// ---------------------------------------------------------------------------
// TestUpdate_RateLimitMsg_*
// ---------------------------------------------------------------------------

// TestUpdate_RateLimitMsg_AddsEntry verifies that a RateLimitMsg produces an
// EventWarning entry whose message contains the provider name and the
// formatted countdown duration.
func TestUpdate_RateLimitMsg_AddsEntry(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	el = sendEventLogMsg(el, RateLimitMsg{
		Provider:   "anthropic",
		ResetAfter: 2 * time.Minute,
		Timestamp:  time.Now(),
	})

	require.Len(t, el.entries, 1, "one entry must be added for RateLimitMsg")
	assert.Equal(t, EventWarning, el.entries[0].Category,
		"RateLimitMsg must produce an EventWarning entry")

	msg := el.entries[0].Message
	assert.Contains(t, msg, "anthropic", "message must contain provider name")
	assert.Contains(t, msg, "2:00", "message must contain formatted countdown '2:00'")
}

// ---------------------------------------------------------------------------
// TestUpdate_ErrorMsg_*
// ---------------------------------------------------------------------------

// TestUpdate_ErrorMsg_AddsEntry verifies that an ErrorMsg with a non-empty
// Detail produces an EventError entry whose message contains the detail.
func TestUpdate_ErrorMsg_AddsEntry(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	el = sendEventLogMsg(el, ErrorMsg{
		Source:    "loop",
		Detail:    "something broke",
		Timestamp: time.Now(),
	})

	require.Len(t, el.entries, 1, "one entry must be added for ErrorMsg")
	assert.Equal(t, EventError, el.entries[0].Category,
		"ErrorMsg must produce an EventError entry")
	assert.Contains(t, el.entries[0].Message, "something broke",
		"message must contain the detail text")
}

// TestUpdate_ErrorMsg_FallsBackToSource verifies that when ErrorMsg.Detail is
// empty the message falls back to the Source field.
func TestUpdate_ErrorMsg_FallsBackToSource(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	el = sendEventLogMsg(el, ErrorMsg{
		Source:    "agent",
		Detail:    "", // empty detail — should use Source
		Timestamp: time.Now(),
	})

	require.Len(t, el.entries, 1)
	assert.Contains(t, el.entries[0].Message, "agent",
		"message must fall back to Source when Detail is empty")
}

// ---------------------------------------------------------------------------
// TestUpdate_FocusChangedMsg_*
// ---------------------------------------------------------------------------

// TestUpdate_FocusChangedMsg_EventLog verifies that a FocusChangedMsg targeting
// FocusEventLog sets el.focused to true.
func TestUpdate_FocusChangedMsg_EventLog(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	require.False(t, el.focused, "focused must be false initially")

	el = sendEventLogMsg(el, FocusChangedMsg{Panel: FocusEventLog})
	assert.True(t, el.focused,
		"focused must be true after FocusChangedMsg{Panel: FocusEventLog}")
}

// TestUpdate_FocusChangedMsg_OtherPanel verifies that a FocusChangedMsg
// targeting a panel other than FocusEventLog sets el.focused to false.
func TestUpdate_FocusChangedMsg_OtherPanel(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	// First focus the event log so we know the transition to false is real.
	el = sendEventLogMsg(el, FocusChangedMsg{Panel: FocusEventLog})
	require.True(t, el.focused)

	el = sendEventLogMsg(el, FocusChangedMsg{Panel: FocusSidebar})
	assert.False(t, el.focused,
		"focused must be false after FocusChangedMsg{Panel: FocusSidebar}")
}

// ---------------------------------------------------------------------------
// TestUpdate_LKey_*
// ---------------------------------------------------------------------------

// TestUpdate_LKey_TogglesVisible verifies that pressing 'l' toggles the
// panel's visibility and pressing it again restores it.
func TestUpdate_LKey_TogglesVisible(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	require.True(t, el.visible, "panel must start visible")

	el, _ = pressEventLogKey(el, 'l')
	assert.False(t, el.visible, "visible must be false after first 'l' press")

	el, _ = pressEventLogKey(el, 'l')
	assert.True(t, el.visible, "visible must be true after second 'l' press")
}

// ---------------------------------------------------------------------------
// TestUpdate_NavKeys_*
// ---------------------------------------------------------------------------

// TestUpdate_NavKeys_WhenFocused verifies that navigation keys are processed
// when the panel is focused: pressing 'k' disables autoScroll, and pressing
// 'G' re-enables it.
func TestUpdate_NavKeys_WhenFocused(t *testing.T) {
	t.Parallel()

	el := makeEventLog(t, 80, 5)
	// Add enough entries to give the viewport content to scroll through.
	for i := 0; i < 30; i++ {
		el.AddEntry(EventInfo, fmt.Sprintf("entry %d", i))
	}

	// Give the panel focus.
	el = sendEventLogMsg(el, FocusChangedMsg{Panel: FocusEventLog})
	require.True(t, el.focused, "panel must be focused before nav key test")
	require.True(t, el.autoScroll, "autoScroll must start true")

	// Scrolling up should disable autoScroll.
	el, _ = pressEventLogKey(el, 'k')
	assert.False(t, el.autoScroll, "autoScroll must be false after pressing 'k'")

	// Pressing 'G' (go to bottom) should re-enable autoScroll.
	el, _ = pressEventLogKey(el, 'G')
	assert.True(t, el.autoScroll, "autoScroll must be true after pressing 'G'")
}

// TestUpdate_NavKeys_WhenUnfocused verifies that navigation keys are ignored
// when the panel does not have focus: autoScroll remains unchanged.
func TestUpdate_NavKeys_WhenUnfocused(t *testing.T) {
	t.Parallel()

	el := makeEventLog(t, 80, 5)
	for i := 0; i < 20; i++ {
		el.AddEntry(EventInfo, fmt.Sprintf("entry %d", i))
	}

	// Ensure the panel is NOT focused.
	el.SetFocused(false)
	require.True(t, el.autoScroll, "autoScroll must start true")

	// Pressing 'k' when unfocused must not affect autoScroll.
	el, _ = pressEventLogKey(el, 'k')
	assert.True(t, el.autoScroll,
		"autoScroll must remain true when 'k' is pressed while unfocused")
}

// ---------------------------------------------------------------------------
// TestClassifyWorkflowEvent_*
// ---------------------------------------------------------------------------

// TestClassifyWorkflowEvent_WithPrevStep verifies the transition format
// "Workflow 'name' step: prevStep -> step" when both steps are provided.
func TestClassifyWorkflowEvent_WithPrevStep(t *testing.T) {
	t.Parallel()

	cat, msg := classifyWorkflowEvent(WorkflowEventMsg{
		WorkflowName: "deploy",
		Step:         "review",
		PrevStep:     "implement",
		Event:        "step_changed",
	})

	assert.Equal(t, EventInfo, cat, "step transition without 'fail'/'error' must be EventInfo")
	assert.Equal(t, "Workflow 'deploy' step: implement -> review", msg,
		"message must follow 'Workflow 'name' step: prevStep -> step' format")
}

// TestClassifyWorkflowEvent_NoPrevStep verifies the format
// "Workflow 'name' step: step" when PrevStep is empty.
func TestClassifyWorkflowEvent_NoPrevStep(t *testing.T) {
	t.Parallel()

	_, msg := classifyWorkflowEvent(WorkflowEventMsg{
		WorkflowName: "deploy",
		Step:         "implement",
		PrevStep:     "",
		Event:        "step_entered",
	})

	assert.Equal(t, "Workflow 'deploy' step: implement", msg,
		"message must follow 'Workflow 'name' step: step' format when PrevStep is empty")
}

// TestClassifyWorkflowEvent_NoSteps verifies the format
// "Workflow 'name' event: event" when both Step and PrevStep are empty.
func TestClassifyWorkflowEvent_NoSteps(t *testing.T) {
	t.Parallel()

	_, msg := classifyWorkflowEvent(WorkflowEventMsg{
		WorkflowName: "deploy",
		Step:         "",
		PrevStep:     "",
		Event:        "initialised",
	})

	assert.Equal(t, "Workflow 'deploy' event: initialised", msg,
		"message must follow 'Workflow 'name' event: event' format when no steps are set")
}

// TestClassifyWorkflowEvent_EmptyName_UsesID verifies that when WorkflowName
// is empty the WorkflowID is used as the name fallback.
func TestClassifyWorkflowEvent_EmptyName_UsesID(t *testing.T) {
	t.Parallel()

	_, msg := classifyWorkflowEvent(WorkflowEventMsg{
		WorkflowID:   "wf-123",
		WorkflowName: "",
		Step:         "review",
		PrevStep:     "",
		Event:        "step_entered",
	})

	assert.Contains(t, msg, "wf-123",
		"message must contain WorkflowID when WorkflowName is empty")
}

// ---------------------------------------------------------------------------
// TestClassifyLoopEvent_*
// ---------------------------------------------------------------------------

// TestClassifyLoopEvent_ErrorWithDetail verifies that a LoopError event with
// a non-empty Detail includes that detail in the message.
func TestClassifyLoopEvent_ErrorWithDetail(t *testing.T) {
	t.Parallel()

	cat, msg := classifyLoopEvent(LoopEventMsg{
		Type:   LoopError,
		Detail: "bad error occurred",
	})

	assert.Equal(t, EventError, cat, "LoopError must produce EventError category")
	assert.Contains(t, msg, "bad error occurred",
		"message must contain the detail text for LoopError")
}

// ---------------------------------------------------------------------------
// TestClassifyAgentStatus_*
// ---------------------------------------------------------------------------

// TestClassifyAgentStatus_FailedWithDetail verifies that an AgentFailed status
// with a non-empty Detail includes that detail in the message.
func TestClassifyAgentStatus_FailedWithDetail(t *testing.T) {
	t.Parallel()

	cat, msg := classifyAgentStatus(AgentStatusMsg{
		Agent:  "claude",
		Status: AgentFailed,
		Detail: "timeout after 30s",
	})

	assert.Equal(t, EventError, cat, "AgentFailed must produce EventError category")
	assert.Contains(t, msg, "timeout after 30s",
		"message must contain detail text for AgentFailed")
}

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

// TestIntegration_600Entries_Only500Retained verifies that adding 600 entries
// (IDs 0..599) results in exactly MaxEventLogEntries (500) entries being
// retained, with entry-100 as the oldest and entry-599 as the newest.
func TestIntegration_600Entries_Only500Retained(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())

	const total = 600
	for i := 0; i < total; i++ {
		el.AddEntry(EventInfo, fmt.Sprintf("entry-%d", i))
	}

	require.Len(t, el.entries, MaxEventLogEntries,
		"entries must be capped at MaxEventLogEntries after adding 600 entries")

	// Oldest retained entry must be entry-100 (entries 0-99 evicted).
	assert.Equal(t, "entry-100", el.entries[0].Message,
		"oldest retained entry must be entry-100")

	// Newest entry must be entry-599.
	assert.Equal(t, "entry-599", el.entries[len(el.entries)-1].Message,
		"newest retained entry must be entry-599")
}

// TestIntegration_AutoScroll_DisabledOnScrollUp verifies the full autoScroll
// lifecycle: starts true, disabled when scrolling up, re-enabled with 'G'.
func TestIntegration_AutoScroll_DisabledOnScrollUp(t *testing.T) {
	t.Parallel()

	el := makeEventLog(t, 80, 5)

	// Add enough entries to create scrollable content.
	for i := 0; i < 50; i++ {
		el.AddEntry(EventInfo, fmt.Sprintf("log entry %d", i))
	}

	assert.True(t, el.autoScroll, "autoScroll must be true after adding entries")

	// Focus the panel and scroll up.
	el = sendEventLogMsg(el, FocusChangedMsg{Panel: FocusEventLog})
	el, _ = pressEventLogKey(el, 'k')
	assert.False(t, el.autoScroll,
		"autoScroll must be false after scrolling up with 'k'")

	// Adding more entries should NOT change autoScroll (entries are appended
	// but the viewport should not jump to the bottom when autoScroll is off).
	for i := 50; i < 60; i++ {
		el.AddEntry(EventInfo, fmt.Sprintf("new entry %d", i))
	}
	assert.False(t, el.autoScroll,
		"autoScroll must remain false after adding more entries while scrolled up")

	// Pressing 'G' must re-enable autoScroll.
	el, _ = pressEventLogKey(el, 'G')
	assert.True(t, el.autoScroll,
		"autoScroll must be true after pressing 'G'")
}

// TestIntegration_VisibilityToggle_PreservesEntries verifies that entries added
// while the panel is hidden are still present when it becomes visible again.
func TestIntegration_VisibilityToggle_PreservesEntries(t *testing.T) {
	t.Parallel()

	el := makeEventLog(t, 80, 20)

	// Add some entries while visible.
	el.AddEntry(EventInfo, "visible-entry-1")
	el.AddEntry(EventInfo, "visible-entry-2")

	// Hide the panel.
	el.SetVisible(false)
	require.False(t, el.IsVisible(), "panel must be hidden")

	// Add more entries while hidden.
	el.AddEntry(EventWarning, "hidden-entry-1")
	el.AddEntry(EventError, "hidden-entry-2")

	// Show the panel again.
	el.SetVisible(true)
	require.True(t, el.IsVisible(), "panel must be visible again")

	// All four entries must be present.
	require.Len(t, el.entries, 4,
		"all entries added (visible or hidden) must be retained after show")

	messages := make([]string, len(el.entries))
	for i, e := range el.entries {
		messages[i] = e.Message
	}

	assert.Contains(t, messages, "visible-entry-1",
		"visible-entry-1 must be retained")
	assert.Contains(t, messages, "visible-entry-2",
		"visible-entry-2 must be retained")
	assert.Contains(t, messages, "hidden-entry-1",
		"hidden-entry-1 must be retained after panel was hidden")
	assert.Contains(t, messages, "hidden-entry-2",
		"hidden-entry-2 must be retained after panel was hidden")

	// The view must render without error and contain entry messages.
	view := stripANSIPanel(el.View())
	assert.NotEmpty(t, view, "View must return non-empty string when visible and has entries")

	// Check that a message string is visible (viewport may show partial content).
	assert.True(t,
		strings.Contains(view, "visible-entry") || strings.Contains(view, "hidden-entry"),
		"View must contain at least one entry message when panel is shown with entries")
}

// ---------------------------------------------------------------------------
// Additional coverage: formatEntry
// ---------------------------------------------------------------------------

// TestFormatEntry_ContainsTimestampAndMessage verifies that formatEntry
// produces a string containing both the HH:MM:SS timestamp and the message.
func TestFormatEntry_ContainsTimestampAndMessage(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	ts := time.Date(2026, 2, 18, 14, 30, 5, 0, time.UTC)
	entry := EventEntry{
		Timestamp: ts,
		Category:  EventInfo,
		Message:   "my event message",
	}

	formatted := stripANSIPanel(el.formatEntry(entry))

	assert.Contains(t, formatted, "14:30:05",
		"formatted entry must contain HH:MM:SS timestamp")
	assert.Contains(t, formatted, "my event message",
		"formatted entry must contain the message text")
}

// ---------------------------------------------------------------------------
// Additional coverage: SetDimensions viewport sizing
// ---------------------------------------------------------------------------

// TestSetDimensions_ViewportHeight verifies that after SetDimensions the
// internal viewport height is (height - 1), reserving one row for the header.
func TestSetDimensions_ViewportHeight(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	el.SetDimensions(80, 20)

	// viewport height = height - 1 = 19.
	assert.Equal(t, 19, el.viewport.Height,
		"viewport height must be height - 1 to reserve one row for the header")
	assert.Equal(t, 80, el.viewport.Width,
		"viewport width must match the panel width")
}

// TestSetDimensions_SmallHeight verifies that a height of 1 results in a
// viewport height of 0 (clamped, not negative).
func TestSetDimensions_SmallHeight(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	el.SetDimensions(80, 1)

	assert.Equal(t, 0, el.viewport.Height,
		"viewport height must be 0 (not negative) when panel height is 1")
}

// ---------------------------------------------------------------------------
// Additional coverage: RateLimitMsg with empty Provider falls back to Agent
// ---------------------------------------------------------------------------

// TestUpdate_RateLimitMsg_EmptyProvider_FallsBackToAgent verifies that when
// RateLimitMsg.Provider is empty the Agent field is used in the message.
func TestUpdate_RateLimitMsg_EmptyProvider_FallsBackToAgent(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	el = sendEventLogMsg(el, RateLimitMsg{
		Provider:   "",
		Agent:      "claude",
		ResetAfter: 30 * time.Second,
		Timestamp:  time.Now(),
	})

	require.Len(t, el.entries, 1)
	assert.Contains(t, el.entries[0].Message, "claude",
		"message must fall back to Agent name when Provider is empty")
}

// ---------------------------------------------------------------------------
// Additional coverage: 'g' key goes to top (disables autoScroll)
// ---------------------------------------------------------------------------

// TestUpdate_gKey_GoesToTop verifies that pressing 'g' when focused scrolls
// to the top and disables autoScroll.
func TestUpdate_gKey_GoesToTop(t *testing.T) {
	t.Parallel()

	el := makeEventLog(t, 80, 5)
	for i := 0; i < 30; i++ {
		el.AddEntry(EventInfo, fmt.Sprintf("entry %d", i))
	}

	el = sendEventLogMsg(el, FocusChangedMsg{Panel: FocusEventLog})
	require.True(t, el.autoScroll)

	el, _ = pressEventLogKey(el, 'g')
	assert.False(t, el.autoScroll,
		"autoScroll must be false after pressing 'g' (go to top)")
}

// ---------------------------------------------------------------------------
// Additional coverage: Up arrow key navigation
// ---------------------------------------------------------------------------

// TestUpdate_UpArrow_WhenFocused_DisablesAutoScroll verifies that pressing the
// Up arrow key when focused disables autoScroll.
func TestUpdate_UpArrow_WhenFocused_DisablesAutoScroll(t *testing.T) {
	t.Parallel()

	el := makeEventLog(t, 80, 5)
	for i := 0; i < 30; i++ {
		el.AddEntry(EventInfo, fmt.Sprintf("entry %d", i))
	}

	el = sendEventLogMsg(el, FocusChangedMsg{Panel: FocusEventLog})
	require.True(t, el.autoScroll)

	el, _ = pressEventLogSpecialKey(el, tea.KeyUp)
	assert.False(t, el.autoScroll,
		"autoScroll must be false after pressing Up arrow key when focused")
}

// TestUpdate_EndKey_WhenFocused_EnablesAutoScroll verifies that pressing the
// End key when focused enables autoScroll.
func TestUpdate_EndKey_WhenFocused_EnablesAutoScroll(t *testing.T) {
	t.Parallel()

	el := makeEventLog(t, 80, 5)
	for i := 0; i < 30; i++ {
		el.AddEntry(EventInfo, fmt.Sprintf("entry %d", i))
	}

	el = sendEventLogMsg(el, FocusChangedMsg{Panel: FocusEventLog})
	// Scroll up first.
	el, _ = pressEventLogKey(el, 'k')
	require.False(t, el.autoScroll, "autoScroll must be false after scrolling up")

	el, _ = pressEventLogSpecialKey(el, tea.KeyEnd)
	assert.True(t, el.autoScroll,
		"autoScroll must be true after pressing End key when focused")
}

// ---------------------------------------------------------------------------
// Additional coverage: multiple messages of different types accumulate
// ---------------------------------------------------------------------------

// TestUpdate_MultipleMessageTypes_AccumulatesEntries verifies that dispatching
// several different message types in sequence adds an entry for each.
func TestUpdate_MultipleMessageTypes_AccumulatesEntries(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())

	el = sendEventLogMsg(el, WorkflowEventMsg{
		WorkflowName: "pipeline",
		Step:         "implement",
		Event:        "step_entered",
		Timestamp:    time.Now(),
	})
	el = sendEventLogMsg(el, LoopEventMsg{
		Type:      LoopIterationStarted,
		Iteration: 1,
		Timestamp: time.Now(),
	})
	el = sendEventLogMsg(el, AgentStatusMsg{
		Agent:     "claude",
		Status:    AgentRunning,
		Timestamp: time.Now(),
	})
	el = sendEventLogMsg(el, RateLimitMsg{
		Provider:   "anthropic",
		ResetAfter: 1 * time.Minute,
		Timestamp:  time.Now(),
	})
	el = sendEventLogMsg(el, ErrorMsg{
		Source:    "loop",
		Detail:    "retry limit",
		Timestamp: time.Now(),
	})

	assert.Len(t, el.entries, 5,
		"five distinct messages must produce five log entries")
}

// ---------------------------------------------------------------------------
// Additional coverage: SetFocused
// ---------------------------------------------------------------------------

// TestSetFocused_UpdatesFocused verifies that SetFocused directly mutates the
// focused field.
func TestSetFocused_UpdatesFocused(t *testing.T) {
	t.Parallel()

	el := NewEventLogModel(DefaultTheme())
	require.False(t, el.focused)

	el.SetFocused(true)
	assert.True(t, el.focused, "focused must be true after SetFocused(true)")

	el.SetFocused(false)
	assert.False(t, el.focused, "focused must be false after SetFocused(false)")
}

// ---------------------------------------------------------------------------
// Additional coverage: EventLog header always renders
// ---------------------------------------------------------------------------

// TestView_Header_AlwaysPresent verifies that the rendered output includes the
// "Event Log" header text both when entries are present and when they are not.
func TestView_Header_AlwaysPresent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		addEntries bool
	}{
		{"no entries", false},
		{"with entries", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			el := makeEventLog(t, 80, 20)
			if tt.addEntries {
				el.AddEntry(EventInfo, "some entry")
			}

			output := stripANSIPanel(el.View())
			assert.Contains(t, output, "Event Log",
				"View must include 'Event Log' header in all non-empty renders")
		})
	}
}
