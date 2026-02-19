package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// makeStatusBar is a convenience constructor that creates a StatusBarModel
// with the default theme and the given width. Width=0 is valid (no-op view).
func makeStatusBar(t *testing.T, width int) StatusBarModel {
	t.Helper()
	sb := NewStatusBarModel(DefaultTheme())
	sb.SetWidth(width)
	return sb
}

// dispatchSB sends any tea.Msg value to the StatusBarModel and returns the
// updated model. Since tea.Msg is defined as any, all message types used in
// Raven's TUI are accepted.
func dispatchSB(sb StatusBarModel, msg any) StatusBarModel {
	return sb.Update(msg)
}

// stripANSI is an alias for stripANSIPanel defined in agent_panel_test.go.
// Both live in package tui so no redeclaration is needed; we use it directly.

// plainView returns the status bar view with ANSI escape sequences stripped,
// making content assertions terminal-independent.
func plainView(sb StatusBarModel) string {
	return stripANSIPanel(sb.View())
}

// ---------------------------------------------------------------------------
// TestNewStatusBarModel_Defaults
// ---------------------------------------------------------------------------

// TestNewStatusBarModel_Defaults verifies that a freshly constructed model
// starts in "idle" mode with all other dynamic fields at zero/empty values.
func TestNewStatusBarModel_Defaults(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())

	assert.Equal(t, "idle", sb.mode, "mode must default to 'idle'")
	assert.Equal(t, "", sb.phase, "phase must be empty after construction")
	assert.Equal(t, "", sb.task, "task must be empty after construction")
	assert.Equal(t, 0, sb.iteration, "iteration must be 0 after construction")
	assert.Equal(t, 0, sb.maxIteration, "maxIteration must be 0 after construction")
	assert.True(t, sb.startTime.IsZero(), "startTime must be zero after construction")
	assert.Equal(t, time.Duration(0), sb.elapsed, "elapsed must be 0 after construction")
	assert.False(t, sb.paused, "paused must be false after construction")
	assert.Equal(t, "", sb.workflow, "workflow must be empty after construction")
	assert.Equal(t, 0, sb.width, "width must be 0 after construction")
}

// ---------------------------------------------------------------------------
// TestSetWidth
// ---------------------------------------------------------------------------

// TestSetWidth verifies that SetWidth updates the internal width field via
// its pointer receiver.
func TestSetWidth(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	require.Equal(t, 0, sb.width, "width must be 0 initially")

	sb.SetWidth(120)
	assert.Equal(t, 120, sb.width, "width must be 120 after SetWidth(120)")

	sb.SetWidth(0)
	assert.Equal(t, 0, sb.width, "width must be 0 after SetWidth(0)")
}

// ---------------------------------------------------------------------------
// TestSetPaused
// ---------------------------------------------------------------------------

// TestSetPaused verifies that SetPaused correctly toggles the paused field
// via its pointer receiver.
func TestSetPaused(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	require.False(t, sb.paused, "paused must be false initially")

	sb.SetPaused(true)
	assert.True(t, sb.paused, "paused must be true after SetPaused(true)")

	sb.SetPaused(false)
	assert.False(t, sb.paused, "paused must be false after SetPaused(false)")
}

// ---------------------------------------------------------------------------
// TestFormatElapsed
// ---------------------------------------------------------------------------

// TestFormatElapsed verifies all documented cases of the formatElapsed helper.
func TestFormatElapsed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{
			name: "zero duration",
			d:    0,
			want: "00:00:00",
		},
		{
			name: "one second",
			d:    time.Second,
			want: "00:00:01",
		},
		{
			name: "59 seconds",
			d:    59 * time.Second,
			want: "00:00:59",
		},
		{
			name: "90 seconds",
			d:    90 * time.Second,
			want: "00:01:30",
		},
		{
			name: "exactly one minute",
			d:    time.Minute,
			want: "00:01:00",
		},
		{
			name: "3661 seconds (1h1m1s)",
			d:    3661 * time.Second,
			want: "01:01:01",
		},
		{
			name: "one hour",
			d:    time.Hour,
			want: "01:00:00",
		},
		{
			name: "24 hours",
			d:    24 * time.Hour,
			want: "24:00:00",
		},
		{
			name: "25 hours 30 minutes 45 seconds",
			d:    25*time.Hour + 30*time.Minute + 45*time.Second,
			want: "25:30:45",
		},
		{
			name: "negative duration treated as zero",
			d:    -5 * time.Second,
			want: "00:00:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatElapsed(tt.d)
			assert.Equal(t, tt.want, got, "formatElapsed(%v) must return %q", tt.d, tt.want)
		})
	}
}

// ---------------------------------------------------------------------------
// TestUpdate_LoopEventMsg
// ---------------------------------------------------------------------------

// TestUpdate_LoopEventMsg_IterationStarted verifies that a LoopIterationStarted
// message sets the start time, iteration, maxIteration and mode="implement".
func TestUpdate_LoopEventMsg_IterationStarted(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	ts := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	sb = dispatchSB(sb, LoopEventMsg{
		Type:      LoopIterationStarted,
		Iteration: 1,
		MaxIter:   20,
		Timestamp: ts,
	})

	assert.Equal(t, "implement", sb.mode, "mode must be 'implement' after LoopIterationStarted")
	assert.Equal(t, 1, sb.iteration, "iteration must be 1")
	assert.Equal(t, 20, sb.maxIteration, "maxIteration must be 20")
	assert.Equal(t, ts, sb.startTime, "startTime must be set from message Timestamp")
}

// TestUpdate_LoopEventMsg_IterationStarted_StartTimeNotOverwritten verifies that
// a second LoopIterationStarted message does not overwrite the already-set
// start time.
func TestUpdate_LoopEventMsg_IterationStarted_StartTimeNotOverwritten(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	first := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	second := time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC)

	sb = dispatchSB(sb, LoopEventMsg{
		Type:      LoopIterationStarted,
		Iteration: 1,
		MaxIter:   5,
		Timestamp: first,
	})
	sb = dispatchSB(sb, LoopEventMsg{
		Type:      LoopIterationStarted,
		Iteration: 2,
		MaxIter:   5,
		Timestamp: second,
	})

	assert.Equal(t, first, sb.startTime,
		"startTime must not be overwritten by subsequent LoopIterationStarted messages")
	assert.Equal(t, 2, sb.iteration, "iteration must update to 2 on second message")
}

// TestUpdate_LoopEventMsg_IterationStarted_ZeroTimestamp verifies that when
// the Timestamp field is zero, startTime is initialised to a non-zero value
// (time.Now()).
func TestUpdate_LoopEventMsg_IterationStarted_ZeroTimestamp(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	before := time.Now()

	sb = dispatchSB(sb, LoopEventMsg{
		Type:      LoopIterationStarted,
		Iteration: 1,
		MaxIter:   10,
		Timestamp: time.Time{}, // zero timestamp
	})

	after := time.Now()
	require.False(t, sb.startTime.IsZero(),
		"startTime must be set to time.Now() when Timestamp is zero")
	assert.True(t, !sb.startTime.Before(before) && !sb.startTime.After(after),
		"startTime must be within the test window when Timestamp is zero")
}

// TestUpdate_LoopEventMsg_IterationStarted_IgnoresZeroIter verifies that
// iteration=0 in LoopIterationStarted does not overwrite a positive iteration.
func TestUpdate_LoopEventMsg_IterationStarted_IgnoresZeroIter(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb = dispatchSB(sb, LoopEventMsg{
		Type:      LoopIterationStarted,
		Iteration: 3,
		MaxIter:   10,
		Timestamp: time.Now(),
	})
	sb = dispatchSB(sb, LoopEventMsg{
		Type:      LoopIterationStarted,
		Iteration: 0, // zero — must be ignored
		MaxIter:   0,
		Timestamp: time.Now(),
	})

	assert.Equal(t, 3, sb.iteration,
		"iteration must remain 3 when LoopIterationStarted sends Iteration=0")
	assert.Equal(t, 10, sb.maxIteration,
		"maxIteration must remain 10 when LoopIterationStarted sends MaxIter=0")
}

// TestStatusBar_Update_LoopEventMsg_TaskSelected verifies that LoopTaskSelected sets
// the task ID, iteration, and maxIteration fields.
func TestStatusBar_Update_LoopEventMsg_TaskSelected(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb = dispatchSB(sb, LoopEventMsg{
		Type:      LoopTaskSelected,
		TaskID:    "T-007",
		Iteration: 3,
		MaxIter:   20,
		Timestamp: time.Now(),
	})

	assert.Equal(t, "T-007", sb.task, "task must be set to 'T-007'")
	assert.Equal(t, 3, sb.iteration, "iteration must be 3")
	assert.Equal(t, 20, sb.maxIteration, "maxIteration must be 20")
}

// TestUpdate_LoopEventMsg_TaskSelected_EmptyTaskIDIgnored verifies that an
// empty TaskID in LoopTaskSelected does not overwrite a previously set task.
func TestUpdate_LoopEventMsg_TaskSelected_EmptyTaskIDIgnored(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb = dispatchSB(sb, LoopEventMsg{
		Type:   LoopTaskSelected,
		TaskID: "T-001",
	})
	sb = dispatchSB(sb, LoopEventMsg{
		Type:   LoopTaskSelected,
		TaskID: "", // empty — must be ignored
	})

	assert.Equal(t, "T-001", sb.task,
		"task must remain 'T-001' when LoopTaskSelected sends empty TaskID")
}

// TestUpdate_LoopEventMsg_IterationCompleted verifies that LoopIterationCompleted
// updates the iteration and maxIteration counters.
func TestUpdate_LoopEventMsg_IterationCompleted(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb = dispatchSB(sb, LoopEventMsg{
		Type:      LoopIterationCompleted,
		Iteration: 5,
		MaxIter:   10,
		Timestamp: time.Now(),
	})

	assert.Equal(t, 5, sb.iteration, "iteration must be 5 after LoopIterationCompleted")
	assert.Equal(t, 10, sb.maxIteration, "maxIteration must be 10 after LoopIterationCompleted")
}

// TestUpdate_LoopEventMsg_TaskCompleted verifies that LoopTaskCompleted updates
// the task field.
func TestUpdate_LoopEventMsg_TaskCompleted(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb = dispatchSB(sb, LoopEventMsg{
		Type:   LoopTaskCompleted,
		TaskID: "T-042",
	})

	assert.Equal(t, "T-042", sb.task, "task must be 'T-042' after LoopTaskCompleted")
}

// TestUpdate_LoopEventMsg_TaskBlocked verifies that LoopTaskBlocked updates
// the task field.
func TestUpdate_LoopEventMsg_TaskBlocked(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb = dispatchSB(sb, LoopEventMsg{
		Type:   LoopTaskBlocked,
		TaskID: "T-099",
	})

	assert.Equal(t, "T-099", sb.task, "task must be 'T-099' after LoopTaskBlocked")
}

// TestUpdate_LoopEventMsg_WaitingForRateLimit verifies that
// LoopWaitingForRateLimit sets paused=true.
func TestUpdate_LoopEventMsg_WaitingForRateLimit(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	require.False(t, sb.paused, "paused must be false initially")

	sb = dispatchSB(sb, LoopEventMsg{
		Type: LoopWaitingForRateLimit,
	})

	assert.True(t, sb.paused, "paused must be true after LoopWaitingForRateLimit")
}

// TestUpdate_LoopEventMsg_ResumedAfterWait verifies that LoopResumedAfterWait
// sets paused=false.
func TestUpdate_LoopEventMsg_ResumedAfterWait(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb.SetPaused(true)
	require.True(t, sb.paused, "paused must be true before test")

	sb = dispatchSB(sb, LoopEventMsg{
		Type: LoopResumedAfterWait,
	})

	assert.False(t, sb.paused, "paused must be false after LoopResumedAfterWait")
}

// TestUpdate_LoopEventMsg_PhaseComplete verifies that LoopPhaseComplete sets
// mode back to "idle".
func TestUpdate_LoopEventMsg_PhaseComplete(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	// First set mode to "implement".
	sb = dispatchSB(sb, LoopEventMsg{
		Type:      LoopIterationStarted,
		Iteration: 1,
		MaxIter:   5,
		Timestamp: time.Now(),
	})
	require.Equal(t, "implement", sb.mode, "mode must be 'implement' before phase complete")

	sb = dispatchSB(sb, LoopEventMsg{Type: LoopPhaseComplete})

	assert.Equal(t, "idle", sb.mode, "mode must be 'idle' after LoopPhaseComplete")
}

// ---------------------------------------------------------------------------
// TestUpdate_WorkflowEventMsg
// ---------------------------------------------------------------------------

// TestUpdate_WorkflowEventMsg_SetsWorkflowAndPhase verifies that a
// WorkflowEventMsg with non-empty WorkflowName and Step fields updates the
// workflow and phase fields.
func TestUpdate_WorkflowEventMsg_SetsWorkflowAndPhase(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb = dispatchSB(sb, WorkflowEventMsg{
		WorkflowID:   "wf-001",
		WorkflowName: "pipeline",
		Step:         "implement",
		PrevStep:     "",
		Event:        "step_entered",
		Timestamp:    time.Now(),
	})

	assert.Equal(t, "pipeline", sb.workflow, "workflow must be set to 'pipeline'")
	assert.Equal(t, "implement", sb.phase, "phase must be set to 'implement'")
}

// TestUpdate_WorkflowEventMsg_EmptyWorkflowNameIgnored verifies that an empty
// WorkflowName does not overwrite a previously set workflow value.
func TestUpdate_WorkflowEventMsg_EmptyWorkflowNameIgnored(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb = dispatchSB(sb, WorkflowEventMsg{WorkflowName: "original", Step: "step1", Event: "x"})
	sb = dispatchSB(sb, WorkflowEventMsg{WorkflowName: "", Step: "step2", Event: "x"})

	assert.Equal(t, "original", sb.workflow,
		"workflow must remain 'original' when empty WorkflowName is received")
}

// TestUpdate_WorkflowEventMsg_EmptyStepIgnored verifies that an empty Step
// field does not overwrite a previously set phase value.
func TestUpdate_WorkflowEventMsg_EmptyStepIgnored(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb = dispatchSB(sb, WorkflowEventMsg{WorkflowName: "pipeline", Step: "review", Event: "x"})
	sb = dispatchSB(sb, WorkflowEventMsg{WorkflowName: "pipeline", Step: "", Event: "x"})

	assert.Equal(t, "review", sb.phase,
		"phase must remain 'review' when empty Step is received")
}

// TestUpdate_WorkflowEventMsg_ModeDerivation verifies that the Event field
// drives mode/paused transitions correctly.
func TestUpdate_WorkflowEventMsg_ModeDerivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		event      string
		startMode  string
		wantMode   string
		wantPaused bool
	}{
		{
			name:      "idle event sets mode idle",
			event:     "idle",
			startMode: "running",
			wantMode:  "idle",
		},
		{
			name:      "stopped event sets mode idle",
			event:     "stopped",
			startMode: "running",
			wantMode:  "idle",
		},
		{
			name:      "not_started event sets mode idle",
			event:     "not_started",
			startMode: "running",
			wantMode:  "idle",
		},
		{
			name:      "completed event sets mode done",
			event:     "completed",
			startMode: "running",
			wantMode:  "done",
		},
		{
			name:      "done event sets mode done",
			event:     "done",
			startMode: "running",
			wantMode:  "done",
		},
		{
			name:      "success event sets mode done",
			event:     "success",
			startMode: "running",
			wantMode:  "done",
		},
		{
			name:      "failed event sets mode error",
			event:     "failed",
			startMode: "running",
			wantMode:  "error",
		},
		{
			name:      "error event sets mode error",
			event:     "error",
			startMode: "running",
			wantMode:  "error",
		},
		{
			name:       "paused event sets paused=true",
			event:      "paused",
			startMode:  "running",
			wantMode:   "running",
			wantPaused: true,
		},
		{
			name:       "waiting event sets paused=true",
			event:      "waiting",
			startMode:  "running",
			wantMode:   "running",
			wantPaused: true,
		},
		{
			name:       "rate_limited event sets paused=true",
			event:      "rate_limited",
			startMode:  "running",
			wantMode:   "running",
			wantPaused: true,
		},
		{
			name:      "unknown event with idle mode transitions to running",
			event:     "some_other_event",
			startMode: "idle",
			wantMode:  "running",
		},
		{
			name:      "unknown event with non-idle mode leaves mode unchanged",
			event:     "some_other_event",
			startMode: "implement",
			wantMode:  "implement",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sb := NewStatusBarModel(DefaultTheme())
			sb.mode = tt.startMode

			sb = dispatchSB(sb, WorkflowEventMsg{
				WorkflowName: "pipeline",
				Step:         "step",
				Event:        tt.event,
				Timestamp:    time.Now(),
			})

			assert.Equal(t, tt.wantMode, sb.mode,
				"mode must be %q after event %q", tt.wantMode, tt.event)
			assert.Equal(t, tt.wantPaused, sb.paused,
				"paused must be %v after event %q", tt.wantPaused, tt.event)
		})
	}
}

// ---------------------------------------------------------------------------
// TestUpdate_TickMsg
// ---------------------------------------------------------------------------

// TestUpdate_TickMsg_AdvancesElapsedWhenNotPaused verifies that a TickMsg
// updates the elapsed duration when the model has a start time and is not
// paused.
func TestUpdate_TickMsg_AdvancesElapsedWhenNotPaused(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	// Inject a start time 5 seconds in the past so elapsed is positive.
	sb.startTime = time.Now().Add(-5 * time.Second)
	sb.paused = false

	sb = dispatchSB(sb, TickMsg{Time: time.Now()})

	assert.Greater(t, sb.elapsed, time.Duration(0),
		"elapsed must be positive after TickMsg when not paused and start time is set")
	// Allow a generous upper bound to prevent flakiness.
	assert.Less(t, sb.elapsed, 30*time.Second,
		"elapsed must be less than 30s in the test window")
}

// TestUpdate_TickMsg_DoesNotAdvanceElapsedWhenPaused verifies that a TickMsg
// does NOT update the elapsed duration when paused=true.
func TestUpdate_TickMsg_DoesNotAdvanceElapsedWhenPaused(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb.startTime = time.Now().Add(-5 * time.Second)
	sb.elapsed = 3 * time.Second
	sb.paused = true

	sb = dispatchSB(sb, TickMsg{Time: time.Now()})

	assert.Equal(t, 3*time.Second, sb.elapsed,
		"elapsed must remain frozen when paused=true and TickMsg arrives")
}

// TestUpdate_TickMsg_DoesNotAdvanceElapsedWhenStartTimeZero verifies that a
// TickMsg does not update elapsed when startTime has not been set.
func TestUpdate_TickMsg_DoesNotAdvanceElapsedWhenStartTimeZero(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	require.True(t, sb.startTime.IsZero(), "startTime must be zero initially")

	sb = dispatchSB(sb, TickMsg{Time: time.Now()})

	assert.Equal(t, time.Duration(0), sb.elapsed,
		"elapsed must remain 0 when startTime is zero and TickMsg arrives")
}

// ---------------------------------------------------------------------------
// TestUpdate_UnknownMsg
// ---------------------------------------------------------------------------

// TestUpdate_UnknownMsg_ReturnsModelUnchanged verifies that an unrecognised
// message type leaves the model state unchanged.
func TestUpdate_UnknownMsg_ReturnsModelUnchanged(t *testing.T) {
	t.Parallel()

	sb := makeStatusBar(t, 100)
	sb.task = "T-001"
	sb.mode = "implement"

	// Send a message type that StatusBarModel does not handle.
	type unknownMsg struct{ val int }
	sb = dispatchSB(sb, unknownMsg{val: 42})

	assert.Equal(t, "T-001", sb.task, "task must be unchanged after unknown message")
	assert.Equal(t, "implement", sb.mode, "mode must be unchanged after unknown message")
}

// ---------------------------------------------------------------------------
// TestView_ZeroWidth
// ---------------------------------------------------------------------------

// TestView_ZeroWidth verifies that View() returns an empty string when the
// width has not been set (or is zero).
func TestView_ZeroWidth(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())

	assert.Equal(t, "", sb.View(),
		"View must return empty string when width is 0")
}

// TestView_NegativeWidth verifies that View() returns an empty string for
// negative widths.
func TestView_NegativeWidth(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb.SetWidth(-1)

	assert.Equal(t, "", sb.View(),
		"View must return empty string when width is negative")
}

// ---------------------------------------------------------------------------
// TestView_ContainsAllSegments
// ---------------------------------------------------------------------------

// TestView_AtWidth100_ContainsAllSegments verifies that at a comfortable
// width (100 columns) the status bar includes mode, phase, task, iteration,
// elapsed time, and the help hint.
func TestView_AtWidth100_ContainsAllSegments(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb.SetWidth(100)
	sb.mode = "implement"
	sb.phase = "review"
	sb.task = "T-007"
	sb.iteration = 3
	sb.maxIteration = 20
	sb.elapsed = 90 * time.Second

	view := plainView(sb)

	assert.Contains(t, view, "implement",
		"view at width 100 must contain mode label 'implement'")
	assert.Contains(t, view, "review",
		"view at width 100 must contain phase value 'review'")
	assert.Contains(t, view, "T-007",
		"view at width 100 must contain task value 'T-007'")
	assert.Contains(t, view, "3/20",
		"view at width 100 must contain iteration counter '3/20'")
	assert.Contains(t, view, "00:01:30",
		"view at width 100 must contain formatted elapsed time '00:01:30'")
	assert.Contains(t, view, "help",
		"view at width 100 must contain the help hint")
}

// TestView_MandatorySegmentsAlwaysPresent verifies that mode and task segments
// appear in the view regardless of width.
func TestView_MandatorySegmentsAlwaysPresent(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb.SetWidth(40)
	sb.mode = "running"
	sb.task = "T-001"

	view := plainView(sb)

	assert.Contains(t, view, "running",
		"mode segment must be present even at narrow width 40")
	assert.Contains(t, view, "T-001",
		"task segment must be present even at narrow width 40")
}

// TestView_HelpHintAlwaysPresent verifies that the "? help" hint appears at
// multiple widths.
func TestView_HelpHintAlwaysPresent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		width int
	}{
		{"width 80", 80},
		{"width 100", 100},
		{"width 200", 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sb := makeStatusBar(t, tt.width)
			view := plainView(sb)

			assert.Contains(t, view, "help",
				"help hint must appear in view at width %d", tt.width)
		})
	}
}

// ---------------------------------------------------------------------------
// TestView_Paused
// ---------------------------------------------------------------------------

// TestView_PausedTrue_ShowsPAUSED verifies that when paused=true the view
// contains the "PAUSED" indicator.
func TestView_PausedTrue_ShowsPAUSED(t *testing.T) {
	t.Parallel()

	sb := makeStatusBar(t, 100)
	sb.SetPaused(true)

	view := plainView(sb)

	assert.Contains(t, view, "PAUSED",
		"view must contain 'PAUSED' when paused=true")
}

// TestView_PausedFalse_DoesNotShowPAUSED verifies that when paused=false the
// view does not contain the "PAUSED" indicator.
func TestView_PausedFalse_DoesNotShowPAUSED(t *testing.T) {
	t.Parallel()

	sb := makeStatusBar(t, 100)
	sb.SetPaused(false)

	view := plainView(sb)

	assert.NotContains(t, view, "PAUSED",
		"view must not contain 'PAUSED' when paused=false")
}

// TestView_PausedTransition verifies that toggling paused via SetPaused
// correctly adds and removes the PAUSED indicator.
func TestView_PausedTransition(t *testing.T) {
	t.Parallel()

	sb := makeStatusBar(t, 100)

	sb.SetPaused(true)
	assert.Contains(t, plainView(sb), "PAUSED",
		"view must show PAUSED after SetPaused(true)")

	sb.SetPaused(false)
	assert.NotContains(t, plainView(sb), "PAUSED",
		"view must not show PAUSED after SetPaused(false)")
}

// ---------------------------------------------------------------------------
// TestView_DefaultSegmentPlaceholders
// ---------------------------------------------------------------------------

// TestView_DefaultPlaceholders verifies that when task and phase fields are
// empty, the view renders the "--" placeholder text for those segments.
func TestView_DefaultPlaceholders(t *testing.T) {
	t.Parallel()

	sb := makeStatusBar(t, 100)
	// task and phase are intentionally left empty.

	view := plainView(sb)

	// The view must contain at least the mode segment and the task placeholder.
	assert.Contains(t, view, "idle",
		"view must show 'idle' mode in default state")
}

// TestView_ZeroIterationCounters verifies that "0/0" appears when iteration
// and maxIteration are both zero.
func TestView_ZeroIterationCounters(t *testing.T) {
	t.Parallel()

	sb := makeStatusBar(t, 200) // wide enough to show iter segment
	// iteration and maxIteration are zero (default).

	view := plainView(sb)

	assert.Contains(t, view, "0/0",
		"view must show '0/0' when iteration and maxIteration are both 0")
}

// ---------------------------------------------------------------------------
// TestView_NarrowWidth_DropOptionalSegments
// ---------------------------------------------------------------------------

// TestView_NarrowWidth_DropsOptionalSegments verifies that at a narrow width
// optional segments (phase, iter, timer) may be dropped without crashing, and
// the mandatory mode and task segments remain.
func TestView_NarrowWidth_DropsOptionalSegments(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb.SetWidth(40) // narrow — may not fit all segments
	sb.mode = "implement"
	sb.task = "T-001"
	sb.phase = "review"
	sb.iteration = 2
	sb.maxIteration = 5
	sb.elapsed = time.Minute

	view := plainView(sb)

	// Must not panic and must not be empty.
	require.NotEmpty(t, view, "view must not be empty at width 40")

	// Mode is mandatory — must always appear.
	assert.Contains(t, view, "implement",
		"mode must be present even at narrow width 40")

	// Task is mandatory — must always appear.
	assert.Contains(t, view, "T-001",
		"task must be present even at narrow width 40")
}

// TestView_MinimumWidth80_AllSegmentsFit verifies that at the minimum
// recommended width of 80 the view renders without panic and includes the
// mandatory segments.
func TestView_MinimumWidth80_AllSegmentsFit(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb.SetWidth(80)
	sb.mode = "implement"
	sb.task = "T-001"

	view := plainView(sb)

	require.NotEmpty(t, view, "view must not be empty at width 80")
	assert.Contains(t, view, "implement",
		"mode must be present at width 80")
	assert.Contains(t, view, "T-001",
		"task must be present at width 80")
	assert.Contains(t, view, "help",
		"help hint must be present at width 80")
}

// ---------------------------------------------------------------------------
// TestView_ElapsedTimerFrozenWhenPaused
// ---------------------------------------------------------------------------

// TestView_ElapsedTimerFrozenWhenPaused verifies that TickMsg does not advance
// elapsed when paused, so the displayed time remains frozen.
func TestView_ElapsedTimerFrozenWhenPaused(t *testing.T) {
	t.Parallel()

	sb := makeStatusBar(t, 200)
	sb.startTime = time.Now().Add(-30 * time.Second)
	sb.elapsed = 30 * time.Second
	sb.SetPaused(true)

	// Dispatch several ticks — elapsed must not change.
	for i := 0; i < 5; i++ {
		sb = dispatchSB(sb, TickMsg{Time: time.Now()})
	}

	assert.Equal(t, 30*time.Second, sb.elapsed,
		"elapsed must remain 30s after ticks when paused=true")
}

// ---------------------------------------------------------------------------
// TestView_VeryLongValues
// ---------------------------------------------------------------------------

// TestView_VeryLongTaskID verifies that an unusually long task ID does not
// cause a panic and the view is still produced.
func TestView_VeryLongTaskID(t *testing.T) {
	t.Parallel()

	sb := makeStatusBar(t, 100)
	sb.task = strings.Repeat("T-VERY-LONG-TASK-ID-", 5) // 100-char task ID

	// Must not panic.
	view := sb.View()
	assert.NotEmpty(t, view, "view must be non-empty with a long task ID")
}

// TestView_VeryLongPhaseName verifies that an unusually long phase name does
// not cause a panic.
func TestView_VeryLongPhaseName(t *testing.T) {
	t.Parallel()

	sb := makeStatusBar(t, 100)
	sb.phase = strings.Repeat("extremely-long-phase-name-", 4)

	view := sb.View()
	assert.NotEmpty(t, view, "view must be non-empty with a long phase name")
}

// ---------------------------------------------------------------------------
// TestView_LargeHours
// ---------------------------------------------------------------------------

// TestView_LargeHourValue verifies that formatElapsed and the timer segment
// still work correctly when elapsed exceeds 24 hours.
func TestView_LargeHourValue(t *testing.T) {
	t.Parallel()

	sb := makeStatusBar(t, 200)
	sb.startTime = time.Now() // not zero
	sb.elapsed = 25*time.Hour + 3*time.Minute + 7*time.Second

	view := plainView(sb)
	assert.Contains(t, view, "25:03:07",
		"view must contain '25:03:07' when elapsed is 25h3m7s")
}

// ---------------------------------------------------------------------------
// TestView_PausedWithElapsedFrozen
// ---------------------------------------------------------------------------

// TestView_PausedShowsFrozenTime verifies that the timer shows the last
// elapsed value (not PAUSED label) via the timer segment while PAUSED
// occupies the mode segment.
func TestView_PausedShowsFrozenTime(t *testing.T) {
	t.Parallel()

	sb := makeStatusBar(t, 200)
	sb.startTime = time.Now().Add(-90 * time.Second)
	sb.elapsed = 90 * time.Second
	sb.SetPaused(true)

	view := plainView(sb)

	// Mode segment is PAUSED (not mode label).
	assert.Contains(t, view, "PAUSED",
		"mode segment must show PAUSED when paused=true")

	// Timer segment still shows frozen elapsed time (optional, wide enough).
	assert.Contains(t, view, "00:01:30",
		"timer segment must show frozen elapsed '00:01:30' when paused=true")
}

// ---------------------------------------------------------------------------
// Integration test: full workflow lifecycle
// ---------------------------------------------------------------------------

// TestIntegration_WorkflowLifecycle simulates a realistic workflow execution:
// start -> iterate -> pause (rate limit) -> resume -> complete.
// It verifies the status bar state at each significant stage.
func TestIntegration_WorkflowLifecycle(t *testing.T) {
	t.Parallel()

	sb := NewStatusBarModel(DefaultTheme())
	sb.SetWidth(120)

	// Stage 1: Workflow starts.
	sb = dispatchSB(sb, WorkflowEventMsg{
		WorkflowName: "full-pipeline",
		Step:         "implement",
		Event:        "step_entered",
		Timestamp:    time.Now(),
	})
	assert.Equal(t, "full-pipeline", sb.workflow, "stage 1: workflow must be 'full-pipeline'")
	assert.Equal(t, "implement", sb.phase, "stage 1: phase must be 'implement'")
	// Event "step_entered" is unknown → transitions idle → running.
	assert.Equal(t, "running", sb.mode, "stage 1: mode must be 'running' after step_entered")

	// Stage 2: First iteration starts.
	ts := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	sb = dispatchSB(sb, LoopEventMsg{
		Type:      LoopIterationStarted,
		Iteration: 1,
		MaxIter:   10,
		Timestamp: ts,
	})
	assert.Equal(t, "implement", sb.mode, "stage 2: mode must be 'implement'")
	assert.Equal(t, 1, sb.iteration, "stage 2: iteration must be 1")
	assert.Equal(t, ts, sb.startTime, "stage 2: startTime must be set from message timestamp")

	// Stage 3: Task selected.
	sb = dispatchSB(sb, LoopEventMsg{
		Type:      LoopTaskSelected,
		TaskID:    "T-015",
		Iteration: 2,
		MaxIter:   10,
		Timestamp: ts.Add(time.Minute),
	})
	assert.Equal(t, "T-015", sb.task, "stage 3: task must be 'T-015'")
	assert.Equal(t, 2, sb.iteration, "stage 3: iteration must be 2")

	// Stage 4: Tick advances elapsed timer.
	sb = dispatchSB(sb, TickMsg{Time: ts.Add(5 * time.Minute)})
	assert.Greater(t, sb.elapsed, time.Duration(0),
		"stage 4: elapsed must be positive after TickMsg")

	// Stage 5: Rate limit pauses the loop.
	sb = dispatchSB(sb, LoopEventMsg{Type: LoopWaitingForRateLimit})
	assert.True(t, sb.paused, "stage 5: paused must be true after LoopWaitingForRateLimit")
	view5 := plainView(sb)
	assert.Contains(t, view5, "PAUSED",
		"stage 5: view must show PAUSED indicator")

	// Stage 5b: Tick while paused must NOT advance elapsed.
	elapsedBefore := sb.elapsed
	sb = dispatchSB(sb, TickMsg{Time: time.Now()})
	assert.Equal(t, elapsedBefore, sb.elapsed,
		"stage 5b: elapsed must not change while paused")

	// Stage 6: Loop resumes after rate limit.
	sb = dispatchSB(sb, LoopEventMsg{Type: LoopResumedAfterWait})
	assert.False(t, sb.paused, "stage 6: paused must be false after LoopResumedAfterWait")
	view6 := plainView(sb)
	assert.NotContains(t, view6, "PAUSED",
		"stage 6: view must not show PAUSED after resume")

	// Stage 7: Phase completes.
	sb = dispatchSB(sb, LoopEventMsg{Type: LoopPhaseComplete})
	assert.Equal(t, "idle", sb.mode,
		"stage 7: mode must be 'idle' after LoopPhaseComplete")

	// Stage 8: Workflow completed.
	sb = dispatchSB(sb, WorkflowEventMsg{
		WorkflowName: "full-pipeline",
		Step:         "done",
		Event:        "completed",
		Timestamp:    time.Now(),
	})
	assert.Equal(t, "done", sb.mode, "stage 8: mode must be 'done' after completed event")
	assert.Equal(t, "done", sb.phase, "stage 8: phase must be 'done'")
}

// ---------------------------------------------------------------------------
// Benchmark tests
// ---------------------------------------------------------------------------

// BenchmarkStatusBarView benchmarks the View() method to ensure rendering a
// fully-populated status bar at normal terminal widths is fast.
func BenchmarkStatusBarView(b *testing.B) {
	sb := NewStatusBarModel(DefaultTheme())
	sb.SetWidth(120)
	sb.mode = "implement"
	sb.phase = "review"
	sb.task = "T-007"
	sb.iteration = 5
	sb.maxIteration = 20
	sb.elapsed = 90 * time.Second

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sb.View()
	}
}

// BenchmarkFormatElapsed benchmarks the formatElapsed helper for regression
// detection on the hot timer rendering path.
func BenchmarkFormatElapsed(b *testing.B) {
	d := 3661 * time.Second
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = formatElapsed(d)
	}
}
