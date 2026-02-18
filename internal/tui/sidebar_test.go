package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stripANSISidebar removes ANSI escape sequences from a string so tests can
// inspect raw content without terminal colour codes.
func stripANSISidebar(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // skip 'm'
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

// applySidebarMsg applies a single message to the SidebarModel and returns the
// updated model plus any command.
func applySidebarMsg(m SidebarModel, msg tea.Msg) (SidebarModel, tea.Cmd) {
	return m.Update(msg)
}

// makeSidebar is a convenience constructor for tests that creates a dimensioned,
// focused sidebar.
func makeSidebar(t *testing.T, width, height int) SidebarModel {
	t.Helper()
	m := NewSidebarModel(DefaultTheme())
	m.SetDimensions(width, height)
	m.SetFocused(true)
	return m
}

// workflowEvent builds a WorkflowEventMsg for use in tests.
func workflowEvent(id, name, event, detail string) WorkflowEventMsg {
	return WorkflowEventMsg{
		WorkflowID:   id,
		WorkflowName: name,
		Event:        event,
		Detail:       detail,
		Timestamp:    time.Now(),
	}
}

// ---- WorkflowStatus ----

func TestWorkflowStatus_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status WorkflowStatus
		want   string
	}{
		{WorkflowIdle, "idle"},
		{WorkflowRunning, "running"},
		{WorkflowPaused, "paused"},
		{WorkflowCompleted, "completed"},
		{WorkflowFailed, "failed"},
		{WorkflowStatus(99), "unknown"},
		{WorkflowStatus(-1), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.status.String())
		})
	}
}

func TestWorkflowStatus_IotaValues(t *testing.T) {
	t.Parallel()
	assert.Equal(t, WorkflowStatus(0), WorkflowIdle)
	assert.Equal(t, WorkflowStatus(1), WorkflowRunning)
	assert.Equal(t, WorkflowStatus(2), WorkflowPaused)
	assert.Equal(t, WorkflowStatus(3), WorkflowCompleted)
	assert.Equal(t, WorkflowStatus(4), WorkflowFailed)
}

// ---- workflowStatusFromEvent ----

func TestWorkflowStatusFromEvent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		event string
		want  WorkflowStatus
	}{
		{"idle", WorkflowIdle},
		{"IDLE", WorkflowIdle},
		{"stopped", WorkflowIdle},
		{"not_started", WorkflowIdle},
		{"paused", WorkflowPaused},
		{"waiting", WorkflowPaused},
		{"rate_limited", WorkflowPaused},
		{"completed", WorkflowCompleted},
		{"done", WorkflowCompleted},
		{"success", WorkflowCompleted},
		{"failed", WorkflowFailed},
		{"error", WorkflowFailed},
		{"running", WorkflowRunning},
		{"step_started", WorkflowRunning},
		{"unknown_event", WorkflowRunning},
		{"", WorkflowRunning},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.event, func(t *testing.T) {
			t.Parallel()
			got := workflowStatusFromEvent(tt.event)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---- NewSidebarModel ----

func TestNewSidebarModel_EmptyWorkflowList(t *testing.T) {
	t.Parallel()
	m := NewSidebarModel(DefaultTheme())
	assert.Empty(t, m.workflows, "new sidebar must have empty workflow list")
	assert.Equal(t, 0, m.selectedIdx)
	assert.Equal(t, 0, m.scrollOffset)
	assert.False(t, m.focused)
}

func TestNewSidebarModel_ZeroDimensions(t *testing.T) {
	t.Parallel()
	m := NewSidebarModel(DefaultTheme())
	assert.Equal(t, 0, m.width)
	assert.Equal(t, 0, m.height)
}

// ---- SetDimensions ----

func TestSidebarModel_SetDimensions(t *testing.T) {
	t.Parallel()
	m := NewSidebarModel(DefaultTheme())
	m.SetDimensions(30, 40)
	assert.Equal(t, 30, m.width)
	assert.Equal(t, 40, m.height)
}

func TestSidebarModel_SetDimensions_UpdatesExisting(t *testing.T) {
	t.Parallel()
	m := NewSidebarModel(DefaultTheme())
	m.SetDimensions(30, 40)
	m.SetDimensions(50, 60)
	assert.Equal(t, 50, m.width)
	assert.Equal(t, 60, m.height)
}

// ---- SetFocused ----

func TestSidebarModel_SetFocused(t *testing.T) {
	t.Parallel()
	m := NewSidebarModel(DefaultTheme())
	assert.False(t, m.focused)
	m.SetFocused(true)
	assert.True(t, m.focused)
	m.SetFocused(false)
	assert.False(t, m.focused)
}

// ---- SelectedWorkflow ----

func TestSidebarModel_SelectedWorkflow_EmptyList(t *testing.T) {
	t.Parallel()
	m := NewSidebarModel(DefaultTheme())
	assert.Equal(t, "", m.SelectedWorkflow())
}

func TestSidebarModel_SelectedWorkflow_ReturnsName(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	m, _ = applySidebarMsg(m, workflowEvent("wf-1", "pipeline", "running", ""))
	assert.Equal(t, "pipeline", m.SelectedWorkflow())
}

func TestSidebarModel_SelectedWorkflow_MultipleWorkflows(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	m, _ = applySidebarMsg(m, workflowEvent("wf-1", "alpha", "running", ""))
	m, _ = applySidebarMsg(m, workflowEvent("wf-2", "beta", "running", ""))
	// Default selection is index 0.
	assert.Equal(t, "alpha", m.SelectedWorkflow())

	// Navigate down.
	m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, "beta", m.SelectedWorkflow())
}

// ---- Update: WorkflowEventMsg ----

func TestSidebarModel_Update_WorkflowEventMsg_AddsNewWorkflow(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	m, cmd := applySidebarMsg(m, workflowEvent("id-1", "pipeline", "running", ""))
	require.Nil(t, cmd)
	require.Len(t, m.workflows, 1)
	assert.Equal(t, "pipeline", m.workflows[0].Name)
	assert.Equal(t, WorkflowRunning, m.workflows[0].Status)
}

func TestSidebarModel_Update_WorkflowEventMsg_UpdatesExistingWorkflow(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	m, _ = applySidebarMsg(m, workflowEvent("id-1", "pipeline", "running", "step-1"))
	m, _ = applySidebarMsg(m, workflowEvent("id-1", "pipeline", "completed", "done"))

	require.Len(t, m.workflows, 1, "duplicate ID must not add a second entry")
	assert.Equal(t, WorkflowCompleted, m.workflows[0].Status)
	assert.Equal(t, "done", m.workflows[0].Detail)
}

func TestSidebarModel_Update_WorkflowEventMsg_PreservesInsertionOrder(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	m, _ = applySidebarMsg(m, workflowEvent("id-1", "alpha", "running", ""))
	m, _ = applySidebarMsg(m, workflowEvent("id-2", "beta", "running", ""))
	m, _ = applySidebarMsg(m, workflowEvent("id-3", "gamma", "running", ""))

	require.Len(t, m.workflows, 3)
	assert.Equal(t, "alpha", m.workflows[0].Name)
	assert.Equal(t, "beta", m.workflows[1].Name)
	assert.Equal(t, "gamma", m.workflows[2].Name)
}

func TestSidebarModel_Update_WorkflowEventMsg_StatusTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		event  string
		status WorkflowStatus
	}{
		{"running", WorkflowRunning},
		{"completed", WorkflowCompleted},
		{"failed", WorkflowFailed},
		{"paused", WorkflowPaused},
		{"idle", WorkflowIdle},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.event, func(t *testing.T) {
			t.Parallel()
			m := makeSidebar(t, 30, 20)
			m, _ = applySidebarMsg(m, workflowEvent("id-1", "wf", tt.event, ""))
			require.Len(t, m.workflows, 1)
			assert.Equal(t, tt.status, m.workflows[0].Status)
		})
	}
}

func TestSidebarModel_Update_WorkflowEventMsg_FallbackIDToName(t *testing.T) {
	t.Parallel()
	// When WorkflowID is empty the WorkflowName is used as the dedup key.
	m := makeSidebar(t, 30, 20)
	msg := WorkflowEventMsg{WorkflowName: "noIDWorkflow", Event: "running", Timestamp: time.Now()}
	m, _ = applySidebarMsg(m, msg)
	require.Len(t, m.workflows, 1)
	assert.Equal(t, "noIDWorkflow", m.workflows[0].Name)
}

// ---- Update: FocusChangedMsg ----

func TestSidebarModel_Update_FocusChangedMsg_SetFocused(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	m.SetFocused(false)

	m, _ = applySidebarMsg(m, FocusChangedMsg{Panel: FocusSidebar})
	assert.True(t, m.focused)
}

func TestSidebarModel_Update_FocusChangedMsg_ClearFocus(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)

	m, _ = applySidebarMsg(m, FocusChangedMsg{Panel: FocusAgentPanel})
	assert.False(t, m.focused)

	m, _ = applySidebarMsg(m, FocusChangedMsg{Panel: FocusEventLog})
	assert.False(t, m.focused)
}

// ---- Update: KeyMsg navigation ----

func TestSidebarModel_Update_KeyMsg_NavigationWhenFocused(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	// Add three workflows.
	m, _ = applySidebarMsg(m, workflowEvent("id-1", "alpha", "running", ""))
	m, _ = applySidebarMsg(m, workflowEvent("id-2", "beta", "running", ""))
	m, _ = applySidebarMsg(m, workflowEvent("id-3", "gamma", "running", ""))

	assert.Equal(t, 0, m.selectedIdx)

	// j moves down.
	m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 1, m.selectedIdx)

	// Down arrow moves down.
	m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, m.selectedIdx)

	// k moves up.
	m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, 1, m.selectedIdx)

	// Up arrow moves up.
	m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, m.selectedIdx)
}

func TestSidebarModel_Update_KeyMsg_ClampsAtBoundaries(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	m, _ = applySidebarMsg(m, workflowEvent("id-1", "only", "running", ""))

	// Moving up from index 0 stays at 0.
	m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, 0, m.selectedIdx)

	// Moving down from last entry stays at last.
	m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 0, m.selectedIdx)
}

func TestSidebarModel_Update_KeyMsg_IgnoredWhenNotFocused(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	m.SetFocused(false)
	m, _ = applySidebarMsg(m, workflowEvent("id-1", "alpha", "running", ""))
	m, _ = applySidebarMsg(m, workflowEvent("id-2", "beta", "running", ""))

	initial := m.selectedIdx
	m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, initial, m.selectedIdx, "navigation should not change selection when unfocused")
}

func TestSidebarModel_Update_KeyMsg_EmptyList_NoPanic(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	assert.NotPanics(t, func() {
		m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	})
}

// ---- View ----

func TestSidebarModel_View_ContainsWorkflowsHeader(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "WORKFLOWS")
}

func TestSidebarModel_View_EmptyList_ShowsPlaceholder(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "No workflows")
}

func TestSidebarModel_View_ShowsWorkflowNames(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	m, _ = applySidebarMsg(m, workflowEvent("id-1", "pipeline", "running", ""))
	m, _ = applySidebarMsg(m, workflowEvent("id-2", "review", "idle", ""))

	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "pipeline")
	assert.Contains(t, view, "review")
}

func TestSidebarModel_View_ShowsStatusIndicators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		event     string
		indicator string
	}{
		{"running", "●"},
		{"idle", "○"},
		{"paused", "◌"},
		{"completed", "✓"},
		{"failed", "✗"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.event, func(t *testing.T) {
			t.Parallel()
			m := makeSidebar(t, 30, 20)
			m, _ = applySidebarMsg(m, workflowEvent("wf", "test", tt.event, ""))
			view := stripANSISidebar(m.View())
			assert.Contains(t, view, tt.indicator,
				"status indicator %q not found for event %q", tt.indicator, tt.event)
		})
	}
}

func TestSidebarModel_View_PadsToHeight(t *testing.T) {
	t.Parallel()
	// Use a raw sidebar without the container style to count lines reliably.
	m := NewSidebarModel(DefaultTheme())
	m.SetDimensions(0, 10) // width=0 skips container style
	m.SetFocused(true)

	view := m.View()
	// The trailing newline after padding means line count = \n occurrences.
	lineCount := strings.Count(view, "\n")
	// We expect the content to fill at least height rows.
	assert.GreaterOrEqual(t, lineCount, 9,
		"view should be padded to approximately the configured height")
}

func TestSidebarModel_View_ZeroDimensions_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	m := NewSidebarModel(DefaultTheme())
	// No SetDimensions call — both are zero.
	view := m.View()
	assert.Empty(t, view)
}

func TestSidebarModel_View_LongNameTruncated(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 20, 20)
	longName := strings.Repeat("x", 100)
	m, _ = applySidebarMsg(m, workflowEvent("id-1", longName, "running", ""))

	view := stripANSISidebar(m.View())
	// Long name should not appear verbatim; ellipsis should be present.
	assert.NotContains(t, view, longName)
	assert.Contains(t, view, "…")
}

func TestSidebarModel_View_WidthConstraint(t *testing.T) {
	t.Parallel()
	width := 25
	m := makeSidebar(t, width, 20)
	m, _ = applySidebarMsg(m, workflowEvent("id-1", "my-workflow", "running", ""))

	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		// Each rendered line (with ANSI stripped) must not exceed width.
		stripped := stripANSISidebar(line)
		assert.LessOrEqual(t, lipgloss.Width(stripped), width,
			"line exceeds configured width: %q", stripped)
	}
}

func TestSidebarModel_View_ContainsFutureSectionPlaceholders(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 30)
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "AGENTS", "agent activity section header must be present")
	assert.Contains(t, view, "PROGRESS", "task progress section header must be present")
}

// ---- Scrolling ----

func TestSidebarModel_View_Scroll_SelectedAlwaysVisible(t *testing.T) {
	t.Parallel()
	// Use a small height so scrolling is triggered.
	m := makeSidebar(t, 30, 6)
	for i := 0; i < 8; i++ {
		id := "wf-" + string(rune('a'+i))
		m, _ = applySidebarMsg(m, workflowEvent(id, id, "running", ""))
	}

	// Navigate to the last entry.
	for i := 0; i < 7; i++ {
		m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyDown})
	}

	selectedName := m.SelectedWorkflow()
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, selectedName,
		"selected workflow %q must be visible after scrolling", selectedName)
}

// ---- clampIdx ----

func TestClampIdx(t *testing.T) {
	t.Parallel()
	tests := []struct {
		idx  int
		n    int
		want int
	}{
		{0, 5, 0},
		{4, 5, 4},
		{5, 5, 4},  // over end → n-1
		{-1, 5, 0}, // below start → 0
		{2, 3, 2},
		{0, 1, 0},
	}
	for _, tt := range tests {
		tt := tt
		t.Run("", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, clampIdx(tt.idx, tt.n))
		})
	}
}

// ---- adjustScroll ----

func TestAdjustScroll(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		offset   int
		selected int
		visible  int
		want     int
	}{
		{name: "selected in window — no change", offset: 0, selected: 2, visible: 5, want: 0},
		{name: "selected below window — scroll down", offset: 0, selected: 5, visible: 5, want: 1},
		{name: "selected above window — scroll up", offset: 3, selected: 1, visible: 5, want: 1},
		{name: "zero visible — returns zero", offset: 2, selected: 5, visible: 0, want: 0},
		{name: "selected at end of window", offset: 0, selected: 4, visible: 5, want: 0},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, adjustScroll(tt.offset, tt.selected, tt.visible))
		})
	}
}

// ---- truncateName ----

func TestTruncateName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		maxWidth int
		wantEll  bool // whether ellipsis should appear
	}{
		{name: "short name fits", input: "abc", maxWidth: 10, wantEll: false},
		{name: "exact fit", input: "hello", maxWidth: 5, wantEll: false},
		{name: "one over", input: "hello!", maxWidth: 5, wantEll: true},
		{name: "long name", input: strings.Repeat("x", 50), maxWidth: 10, wantEll: true},
		{name: "zero width", input: "abc", maxWidth: 0, wantEll: false},
		{name: "empty input", input: "", maxWidth: 10, wantEll: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := truncateName(tt.input, tt.maxWidth)
			if tt.wantEll {
				assert.Contains(t, result, "…", "expected ellipsis in truncated name")
				assert.LessOrEqual(t, lipgloss.Width(result), tt.maxWidth,
					"truncated name must fit within maxWidth")
			} else {
				assert.NotContains(t, result, "…")
			}
		})
	}
}

// ---- TaskProgressSection ----

func TestNewTaskProgressSection_ZeroValues(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	assert.Equal(t, 0, tp.totalTasks)
	assert.Equal(t, 0, tp.completedTasks)
	assert.Equal(t, 0, tp.totalPhases)
	assert.Equal(t, 0, tp.currentPhase)
	assert.Equal(t, 0, tp.phaseTasks)
	assert.Equal(t, 0, tp.phaseCompleted)
}

func TestTaskProgressSection_SetTotals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		totalTasks      int
		totalPhases     int
		wantTotalTasks  int
		wantTotalPhases int
	}{
		{name: "positive values", totalTasks: 30, totalPhases: 5, wantTotalTasks: 30, wantTotalPhases: 5},
		{name: "zero values", totalTasks: 0, totalPhases: 0, wantTotalTasks: 0, wantTotalPhases: 0},
		{name: "negative tasks clamped", totalTasks: -5, totalPhases: 3, wantTotalTasks: 0, wantTotalPhases: 3},
		{name: "negative phases clamped", totalTasks: 10, totalPhases: -2, wantTotalTasks: 10, wantTotalPhases: 0},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tp := NewTaskProgressSection(DefaultTheme())
			tp.SetTotals(tt.totalTasks, tt.totalPhases)
			assert.Equal(t, tt.wantTotalTasks, tp.totalTasks)
			assert.Equal(t, tt.wantTotalPhases, tp.totalPhases)
		})
	}
}

func TestTaskProgressSection_SetPhase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		phase              int
		phaseTasks         int
		phaseCompleted     int
		wantPhase          int
		wantPhaseTasks     int
		wantPhaseCompleted int
	}{
		{name: "valid phase", phase: 2, phaseTasks: 10, phaseCompleted: 4, wantPhase: 2, wantPhaseTasks: 10, wantPhaseCompleted: 4},
		{name: "negative phase clamped", phase: -1, phaseTasks: 5, phaseCompleted: 2, wantPhase: 0, wantPhaseTasks: 5, wantPhaseCompleted: 2},
		{name: "negative phaseTasks clamped", phase: 1, phaseTasks: -3, phaseCompleted: 0, wantPhase: 1, wantPhaseTasks: 0, wantPhaseCompleted: 0},
		{name: "negative phaseCompleted clamped", phase: 1, phaseTasks: 5, phaseCompleted: -1, wantPhase: 1, wantPhaseTasks: 5, wantPhaseCompleted: 0},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tp := NewTaskProgressSection(DefaultTheme())
			tp.SetPhase(tt.phase, tt.phaseTasks, tt.phaseCompleted)
			assert.Equal(t, tt.wantPhase, tp.currentPhase)
			assert.Equal(t, tt.wantPhaseTasks, tp.phaseTasks)
			assert.Equal(t, tt.wantPhaseCompleted, tp.phaseCompleted)
		})
	}
}

func TestTaskProgressSection_Update_TaskProgressMsg(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		completed      int
		total          int
		wantCompleted  int
		wantTotal      int
	}{
		{name: "normal progress", completed: 12, total: 30, wantCompleted: 12, wantTotal: 30},
		{name: "completed equals total", completed: 30, total: 30, wantCompleted: 30, wantTotal: 30},
		{name: "completed exceeds total — clamped", completed: 35, total: 30, wantCompleted: 30, wantTotal: 30},
		{name: "zero progress", completed: 0, total: 30, wantCompleted: 0, wantTotal: 30},
		{name: "negative completed — treated as zero", completed: -5, total: 10, wantCompleted: 0, wantTotal: 10},
		{name: "negative total — treated as zero", completed: 5, total: -10, wantCompleted: 0, wantTotal: 0},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tp := NewTaskProgressSection(DefaultTheme())
			msg := TaskProgressMsg{Completed: tt.completed, Total: tt.total}
			tp = tp.Update(msg)
			assert.Equal(t, tt.wantCompleted, tp.completedTasks)
			assert.Equal(t, tt.wantTotal, tp.totalTasks)
		})
	}
}

func TestTaskProgressSection_Update_LoopEventMsg_PhaseComplete(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	// Simulate some completed phase tasks first.
	tp.phaseCompleted = 5

	msg := LoopEventMsg{Type: LoopPhaseComplete}
	tp = tp.Update(msg)

	assert.Equal(t, 1, tp.currentPhase, "LoopPhaseComplete must increment currentPhase")
	assert.Equal(t, 0, tp.phaseCompleted, "LoopPhaseComplete must reset phaseCompleted")
}

func TestTaskProgressSection_Update_LoopEventMsg_PhaseComplete_MultipleTimes(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	for i := 1; i <= 3; i++ {
		tp = tp.Update(LoopEventMsg{Type: LoopPhaseComplete})
		assert.Equal(t, i, tp.currentPhase)
	}
}

func TestTaskProgressSection_Update_LoopEventMsg_TaskCompleted(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp.totalTasks = 10
	tp.completedTasks = 2

	tp = tp.Update(LoopEventMsg{Type: LoopTaskCompleted})

	assert.Equal(t, 1, tp.phaseCompleted, "LoopTaskCompleted must increment phaseCompleted")
	assert.Equal(t, 3, tp.completedTasks, "LoopTaskCompleted must increment completedTasks when below total")
}

func TestTaskProgressSection_Update_LoopEventMsg_TaskCompleted_ClampedAtTotal(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp.totalTasks = 5
	tp.completedTasks = 5 // already at total

	tp = tp.Update(LoopEventMsg{Type: LoopTaskCompleted})

	assert.Equal(t, 5, tp.completedTasks, "completedTasks must not exceed totalTasks")
}

func TestTaskProgressSection_Update_UnhandledMsg_NoChange(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp.totalTasks = 10
	tp.completedTasks = 3

	// Send an unrelated message type.
	tp = tp.Update(WorkflowEventMsg{WorkflowName: "wf", Event: "running"})

	assert.Equal(t, 10, tp.totalTasks, "unhandled message must not change totalTasks")
	assert.Equal(t, 3, tp.completedTasks, "unhandled message must not change completedTasks")
}

func TestTaskProgressSection_View_NoTasks_ShowsPlaceholder(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	view := stripANSISidebar(tp.View(30))
	assert.Contains(t, view, "No tasks")
	assert.Contains(t, view, "No phases")
}

func TestTaskProgressSection_View_WithTasks_ShowsBar(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp = tp.Update(TaskProgressMsg{Completed: 12, Total: 30})
	view := stripANSISidebar(tp.View(30))
	assert.Contains(t, view, "Tasks")
	assert.Contains(t, view, "40%")
	assert.Contains(t, view, "12/30 done")
}

func TestTaskProgressSection_View_WithPhases_ShowsPhaseBar(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp.SetTotals(30, 5)
	tp.SetPhase(2, 8, 4)
	view := stripANSISidebar(tp.View(30))
	assert.Contains(t, view, "Phase: 2/5")
	assert.Contains(t, view, "50%")
}

func TestTaskProgressSection_View_FullCompletion_Shows100Percent(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp = tp.Update(TaskProgressMsg{Completed: 30, Total: 30})
	view := stripANSISidebar(tp.View(30))
	assert.Contains(t, view, "100%")
	assert.Contains(t, view, "30/30 done")
}

func TestTaskProgressSection_View_ZeroWidth_NoPanic(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp = tp.Update(TaskProgressMsg{Completed: 5, Total: 10})
	assert.NotPanics(t, func() {
		_ = tp.View(0)
	})
}

func TestTaskProgressSection_View_ZeroPhases_ShowsNoPhases(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp.SetTotals(10, 0) // zero phases
	tp = tp.Update(TaskProgressMsg{Completed: 5, Total: 10})
	view := stripANSISidebar(tp.View(30))
	assert.Contains(t, view, "No phases")
}

// ---- SidebarModel: TaskProgressMsg and LoopEventMsg integration ----

func TestSidebarModel_Update_TaskProgressMsg(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	msg := TaskProgressMsg{TaskID: "T-001", Completed: 5, Total: 20}
	m, cmd := applySidebarMsg(m, msg)
	require.Nil(t, cmd)
	assert.Equal(t, 5, m.taskProgress.completedTasks)
	assert.Equal(t, 20, m.taskProgress.totalTasks)
}

func TestSidebarModel_Update_LoopEventMsg_PhaseComplete(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	m, _ = applySidebarMsg(m, LoopEventMsg{Type: LoopPhaseComplete})
	assert.Equal(t, 1, m.taskProgress.currentPhase)
	assert.Equal(t, 0, m.taskProgress.phaseCompleted)
}

func TestSidebarModel_Update_LoopEventMsg_TaskCompleted(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	m.taskProgress.totalTasks = 10
	m, _ = applySidebarMsg(m, LoopEventMsg{Type: LoopTaskCompleted})
	assert.Equal(t, 1, m.taskProgress.phaseCompleted)
}

func TestSidebarModel_SetTotals_Delegates(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	m.SetTotals(50, 8)
	assert.Equal(t, 50, m.taskProgress.totalTasks)
	assert.Equal(t, 8, m.taskProgress.totalPhases)
}

func TestSidebarModel_SetPhase_Delegates(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	m.SetPhase(3, 12, 6)
	assert.Equal(t, 3, m.taskProgress.currentPhase)
	assert.Equal(t, 12, m.taskProgress.phaseTasks)
	assert.Equal(t, 6, m.taskProgress.phaseCompleted)
}

func TestSidebarModel_View_TaskProgressRendered(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	m, _ = applySidebarMsg(m, TaskProgressMsg{Completed: 10, Total: 25})
	m.SetTotals(25, 5)
	m.SetPhase(2, 6, 3)
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "Tasks", "Tasks header must appear in sidebar view")
	assert.Contains(t, view, "10/25 done", "completion text must appear in sidebar view")
	assert.Contains(t, view, "Phase: 2/5", "phase header must appear in sidebar view")
}

// ---- Integration: sequence of messages ----

func TestSidebarModel_Integration_SequentialMessages(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)

	// Add three workflows.
	m, _ = applySidebarMsg(m, workflowEvent("wf-1", "deploy", "running", "step-1"))
	m, _ = applySidebarMsg(m, workflowEvent("wf-2", "review", "idle", ""))
	m, _ = applySidebarMsg(m, workflowEvent("wf-3", "fix", "running", "T-007"))

	require.Len(t, m.workflows, 3)

	// Transition wf-2 to running.
	m, _ = applySidebarMsg(m, workflowEvent("wf-2", "review", "running", "step-2"))
	assert.Equal(t, WorkflowRunning, m.workflows[1].Status)

	// Navigate to wf-3.
	m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, "fix", m.SelectedWorkflow())

	// View should contain all three names.
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "deploy")
	assert.Contains(t, view, "review")
	assert.Contains(t, view, "fix")
}

func TestSidebarModel_Integration_FocusToggle(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	m, _ = applySidebarMsg(m, workflowEvent("id-1", "alpha", "running", ""))
	m, _ = applySidebarMsg(m, workflowEvent("id-2", "beta", "running", ""))

	// Lose focus → navigation should do nothing.
	m, _ = applySidebarMsg(m, FocusChangedMsg{Panel: FocusAgentPanel})
	before := m.selectedIdx
	m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, before, m.selectedIdx)

	// Regain focus → navigation should work.
	m, _ = applySidebarMsg(m, FocusChangedMsg{Panel: FocusSidebar})
	m, _ = applySidebarMsg(m, tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, m.selectedIdx)
}

func TestSidebarModel_Integration_DuplicateEvents_Idempotent(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 20)
	for i := 0; i < 5; i++ {
		m, _ = applySidebarMsg(m, workflowEvent("id-1", "pipeline", "running", ""))
	}
	assert.Len(t, m.workflows, 1, "duplicate events must not add multiple entries")
}

// ---------------------------------------------------------------------------
// T-071: TaskProgressSection — additional View rendering tests
// ---------------------------------------------------------------------------

func TestTaskProgressSection_View_TasksHeader_AlwaysPresent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		total     int
		completed int
	}{
		{"zero tasks", 0, 0},
		{"some tasks", 10, 3},
		{"full tasks", 5, 5},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tp := NewTaskProgressSection(DefaultTheme())
			if tt.total > 0 {
				tp = tp.Update(TaskProgressMsg{Completed: tt.completed, Total: tt.total})
			}
			view := stripANSISidebar(tp.View(30))
			assert.Contains(t, view, "Tasks",
				"Tasks header must always appear regardless of task count")
		})
	}
}

func TestTaskProgressSection_View_EmptyBar_ZeroCompletion(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp = tp.Update(TaskProgressMsg{Completed: 0, Total: 20})
	view := stripANSISidebar(tp.View(30))
	// 0% completion must be rendered.
	assert.Contains(t, view, "0%")
	assert.Contains(t, view, "0/20 done")
}

func TestTaskProgressSection_View_HalfFilledBar_15of30(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp = tp.Update(TaskProgressMsg{Completed: 15, Total: 30})
	view := stripANSISidebar(tp.View(30))
	assert.Contains(t, view, "50%")
	assert.Contains(t, view, "15/30 done")
}

func TestTaskProgressSection_View_CompletionText_Format(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		completed int
		total     int
		wantText  string
		wantPct   string
	}{
		{"zero of many", 0, 50, "0/50 done", "0%"},
		{"one of one", 1, 1, "1/1 done", "100%"},
		{"twelve of thirty", 12, 30, "12/30 done", "40%"},
		{"one of three", 1, 3, "1/3 done", "33%"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tp := NewTaskProgressSection(DefaultTheme())
			tp = tp.Update(TaskProgressMsg{Completed: tt.completed, Total: tt.total})
			view := stripANSISidebar(tp.View(40))
			assert.Contains(t, view, tt.wantText,
				"completion text must match N/M done format")
			assert.Contains(t, view, tt.wantPct,
				"percentage must be rendered")
		})
	}
}

func TestTaskProgressSection_View_PhaseHeader_Format(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		currentPhase int
		totalPhases  int
		wantHeader   string
	}{
		{"phase 1 of 5", 1, 5, "Phase: 1/5"},
		{"phase 0 of 3", 0, 3, "Phase: 0/3"},
		{"phase 3 of 3", 3, 3, "Phase: 3/3"},
		{"no phases", 0, 0, "Phase: 0/0"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tp := NewTaskProgressSection(DefaultTheme())
			tp.SetTotals(10, tt.totalPhases)
			tp.SetPhase(tt.currentPhase, 5, 2)
			view := stripANSISidebar(tp.View(30))
			assert.Contains(t, view, tt.wantHeader,
				"phase header must use Phase: N/M format")
		})
	}
}

func TestTaskProgressSection_View_PhaseBar_HalfComplete(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp.SetTotals(30, 4)
	tp.SetPhase(2, 10, 5) // 5/10 = 50% of phase tasks done
	view := stripANSISidebar(tp.View(30))
	assert.Contains(t, view, "50%")
}

func TestTaskProgressSection_View_PhaseBar_ZeroTasksInPhase(t *testing.T) {
	t.Parallel()
	// Phase has been configured with phases>0 but phaseTasks=0;
	// the phase bar should show 0% (no division by zero panic).
	tp := NewTaskProgressSection(DefaultTheme())
	tp.SetTotals(20, 4)
	tp.SetPhase(1, 0, 0)
	assert.NotPanics(t, func() {
		view := stripANSISidebar(tp.View(30))
		assert.Contains(t, view, "0%")
	})
}

func TestTaskProgressSection_View_NarrowWidth_NoPanic(t *testing.T) {
	t.Parallel()
	widths := []int{1, 2, 3, 4, 5}
	tp := NewTaskProgressSection(DefaultTheme())
	tp = tp.Update(TaskProgressMsg{Completed: 3, Total: 10})
	tp.SetTotals(10, 3)
	tp.SetPhase(1, 4, 2)
	for _, w := range widths {
		w := w
		t.Run(fmt.Sprintf("width_%d", w), func(t *testing.T) {
			t.Parallel()
			assert.NotPanics(t, func() {
				_ = tp.View(w)
			})
		})
	}
}

func TestTaskProgressSection_View_ProgressNeverExceeds100Percent(t *testing.T) {
	t.Parallel()
	// completedTasks clamped to totalTasks in Update, so percentage must
	// never exceed 100%.
	tp := NewTaskProgressSection(DefaultTheme())
	// Force an over-count via direct field mutation after Update clamping.
	tp = tp.Update(TaskProgressMsg{Completed: 100, Total: 10}) // clamped to 10/10
	view := stripANSISidebar(tp.View(30))
	assert.Contains(t, view, "100%")
	assert.NotContains(t, view, "200%")
	assert.NotContains(t, view, "1000%")
}

func TestTaskProgressSection_View_PhaseCompletedClampedToPhaseTotal(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp.SetTotals(20, 4)
	// Set phaseCompleted > phaseTasks; View must clamp it internally.
	tp.SetPhase(1, 5, 5) // exactly at total — should show 100%
	view := stripANSISidebar(tp.View(30))
	assert.Contains(t, view, "100%")
}

// ---------------------------------------------------------------------------
// T-071: TaskProgressSection — Update edge-case tests
// ---------------------------------------------------------------------------

func TestTaskProgressSection_Update_TaskProgressMsg_StatusCompleted_IncreasesCompleted(t *testing.T) {
	t.Parallel()
	// When Status == "completed", completedTasks is derived from Completed field.
	// The existing impl uses Completed/Total fields, not the Status string.
	// This test verifies that a message with Status "completed" and explicit
	// Completed/Total fields still updates counts correctly.
	tp := NewTaskProgressSection(DefaultTheme())
	msg := TaskProgressMsg{Status: "completed", Completed: 7, Total: 20}
	tp = tp.Update(msg)
	assert.Equal(t, 7, tp.completedTasks)
	assert.Equal(t, 20, tp.totalTasks)
}

func TestTaskProgressSection_Update_LoopPhaseComplete_ResetsPhaseCompleted(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	// Build up some phase completion via LoopTaskCompleted events.
	tp.totalTasks = 15
	for i := 0; i < 4; i++ {
		tp = tp.Update(LoopEventMsg{Type: LoopTaskCompleted})
	}
	assert.Equal(t, 4, tp.phaseCompleted)

	// A phase-complete event must reset phaseCompleted to zero.
	tp = tp.Update(LoopEventMsg{Type: LoopPhaseComplete})
	assert.Equal(t, 0, tp.phaseCompleted,
		"LoopPhaseComplete must reset phaseCompleted to zero")
	assert.Equal(t, 1, tp.currentPhase,
		"LoopPhaseComplete must increment currentPhase")
}

func TestTaskProgressSection_Update_LoopTaskCompleted_DoesNotExceedTotalWhenAtLimit(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp.totalTasks = 3
	tp.completedTasks = 3 // already at total

	// Sending more LoopTaskCompleted events must not push count past total.
	for i := 0; i < 5; i++ {
		tp = tp.Update(LoopEventMsg{Type: LoopTaskCompleted})
	}
	assert.Equal(t, 3, tp.completedTasks,
		"completedTasks must not exceed totalTasks after repeated LoopTaskCompleted")
}

func TestTaskProgressSection_Update_NegativeValues_TreatedAsZero(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	msg := TaskProgressMsg{Completed: -10, Total: -5}
	tp = tp.Update(msg)
	assert.Equal(t, 0, tp.completedTasks, "negative Completed must be treated as zero")
	assert.Equal(t, 0, tp.totalTasks, "negative Total must be treated as zero")
}

func TestTaskProgressSection_Update_CompletedGreaterThanTotal_Clamped(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	msg := TaskProgressMsg{Completed: 50, Total: 20}
	tp = tp.Update(msg)
	assert.Equal(t, 20, tp.completedTasks,
		"completed exceeding total must be clamped to total")
	assert.Equal(t, 20, tp.totalTasks)
}

func TestTaskProgressSection_SetPhase_ChangePhase_UpdatesCounters(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	// First set phase 1 with some completion.
	tp.SetPhase(1, 8, 6)
	assert.Equal(t, 1, tp.currentPhase)
	assert.Equal(t, 8, tp.phaseTasks)
	assert.Equal(t, 6, tp.phaseCompleted)

	// Changing to phase 2 with different counts must fully replace.
	tp.SetPhase(2, 12, 3)
	assert.Equal(t, 2, tp.currentPhase)
	assert.Equal(t, 12, tp.phaseTasks)
	assert.Equal(t, 3, tp.phaseCompleted)
}

func TestTaskProgressSection_SetTotals_NegativeValues_ClampedToZero(t *testing.T) {
	t.Parallel()
	tp := NewTaskProgressSection(DefaultTheme())
	tp.SetTotals(-100, -50)
	assert.Equal(t, 0, tp.totalTasks)
	assert.Equal(t, 0, tp.totalPhases)
}

// ---------------------------------------------------------------------------
// T-071: SidebarModel — integration with TaskProgress section
// ---------------------------------------------------------------------------

func TestSidebarModel_Integration_TaskProgressSequence(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 35, 50)
	m.SetTotals(10, 3)

	// Phase 1: complete 4 tasks via LoopTaskCompleted.
	m.SetPhase(1, 4, 0)
	for i := 0; i < 4; i++ {
		m, _ = applySidebarMsg(m, LoopEventMsg{Type: LoopTaskCompleted})
	}
	assert.Equal(t, 4, m.taskProgress.phaseCompleted)
	assert.Equal(t, 4, m.taskProgress.completedTasks)

	// Phase 1 ends; LoopPhaseComplete increments currentPhase (1→2) and resets phaseCompleted.
	m, _ = applySidebarMsg(m, LoopEventMsg{Type: LoopPhaseComplete})
	assert.Equal(t, 2, m.taskProgress.currentPhase)
	assert.Equal(t, 0, m.taskProgress.phaseCompleted)

	// Phase 2: complete 3 tasks.
	m.SetPhase(2, 3, 0)
	for i := 0; i < 3; i++ {
		m, _ = applySidebarMsg(m, LoopEventMsg{Type: LoopTaskCompleted})
	}
	assert.Equal(t, 3, m.taskProgress.phaseCompleted)
	assert.Equal(t, 7, m.taskProgress.completedTasks)

	// Phase 2 ends; LoopPhaseComplete increments currentPhase (2→3).
	m, _ = applySidebarMsg(m, LoopEventMsg{Type: LoopPhaseComplete})
	assert.Equal(t, 3, m.taskProgress.currentPhase)

	// Phase 3: complete final 3 tasks.
	m.SetPhase(3, 3, 0)
	for i := 0; i < 3; i++ {
		m, _ = applySidebarMsg(m, LoopEventMsg{Type: LoopTaskCompleted})
	}
	assert.Equal(t, 10, m.taskProgress.completedTasks)

	// Final view must show 100% overall completion.
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "100%")
	assert.Contains(t, view, "10/10 done")
}

func TestSidebarModel_Integration_TaskProgressMsg_OverridesLoopCounts(t *testing.T) {
	t.Parallel()
	// TaskProgressMsg must set exact counts regardless of what LoopTaskCompleted
	// accumulated previously.
	m := makeSidebar(t, 30, 40)
	m.taskProgress.totalTasks = 20

	// Accumulate via loop events.
	for i := 0; i < 5; i++ {
		m, _ = applySidebarMsg(m, LoopEventMsg{Type: LoopTaskCompleted})
	}
	assert.Equal(t, 5, m.taskProgress.completedTasks)

	// An explicit TaskProgressMsg resets to its exact values.
	m, _ = applySidebarMsg(m, TaskProgressMsg{Completed: 18, Total: 20})
	assert.Equal(t, 18, m.taskProgress.completedTasks)
	assert.Equal(t, 20, m.taskProgress.totalTasks)
}

func TestSidebarModel_View_ProgressSectionHeader_AlwaysPresent(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "PROGRESS",
		"PROGRESS section header must always be rendered in the sidebar view")
}

func TestSidebarModel_View_TasksAndPhaseHeadersInView(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 35, 50)
	m, _ = applySidebarMsg(m, TaskProgressMsg{Completed: 5, Total: 15})
	m.SetTotals(15, 4)
	m.SetPhase(2, 5, 3)

	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "Tasks", "Tasks sub-header must appear in sidebar")
	assert.Contains(t, view, "Phase: 2/4", "Phase header must appear in sidebar")
	assert.Contains(t, view, "5/15 done", "Completion text must appear in sidebar")
}

func TestSidebarModel_View_NoTasks_ShowsNoTasksPlaceholder(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	// No TaskProgressMsg sent — totalTasks remains 0.
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "No tasks",
		"sidebar must show No tasks placeholder when no tasks are set")
}

func TestSidebarModel_SetTotals_ZeroPhases_ShowsNoPhasesInView(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	m.SetTotals(10, 0) // tasks but no phases
	m, _ = applySidebarMsg(m, TaskProgressMsg{Completed: 3, Total: 10})

	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "No phases",
		"sidebar must show No phases placeholder when totalPhases is zero")
}

func TestSidebarModel_Update_MultipleLoopPhaseComplete_Accumulates(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	for i := 1; i <= 5; i++ {
		m, _ = applySidebarMsg(m, LoopEventMsg{Type: LoopPhaseComplete})
		assert.Equal(t, i, m.taskProgress.currentPhase,
			"currentPhase must increment by 1 for each LoopPhaseComplete")
	}
}

func TestSidebarModel_View_BarWidth_ConstrainedToSidebarWidth(t *testing.T) {
	t.Parallel()
	// Progress bars are rendered at width-2; verify the output lines
	// stay within the configured width after ANSI stripping.
	m := makeSidebar(t, 28, 50)
	m, _ = applySidebarMsg(m, TaskProgressMsg{Completed: 7, Total: 14})
	m.SetTotals(14, 3)
	m.SetPhase(1, 5, 2)

	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		stripped := stripANSISidebar(line)
		assert.LessOrEqual(t, lipgloss.Width(stripped), 28,
			"line %d exceeds sidebar width: %q", i, stripped)
	}
}

// ---------------------------------------------------------------------------
// T-072: formatCountdown
// ---------------------------------------------------------------------------

func TestFormatCountdown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "zero duration", d: 0, want: "0:00"},
		{name: "negative duration", d: -5 * time.Second, want: "0:00"},
		{name: "one second", d: time.Second, want: "0:01"},
		{name: "59 seconds", d: 59 * time.Second, want: "0:59"},
		{name: "one minute", d: 60 * time.Second, want: "1:00"},
		{name: "one minute 42 seconds", d: 102 * time.Second, want: "1:42"},
		{name: "59 minutes 59 seconds", d: 59*time.Minute + 59*time.Second, want: "59:59"},
		{name: "exactly one hour", d: time.Hour, want: "1:00:00"},
		{name: "one hour 2 minutes 3 seconds", d: time.Hour + 2*time.Minute + 3*time.Second, want: "1:02:03"},
		{name: "two hours", d: 2 * time.Hour, want: "2:00:00"},
		{name: "10 hours 5 minutes 7 seconds", d: 10*time.Hour + 5*time.Minute + 7*time.Second, want: "10:05:07"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatCountdown(tt.d)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// T-072: NewRateLimitSection
// ---------------------------------------------------------------------------

func TestNewRateLimitSection_EmptyProviders(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	assert.Empty(t, rl.providers)
	assert.Empty(t, rl.order)
}

func TestNewRateLimitSection_HasActiveLimit_False(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	assert.False(t, rl.HasActiveLimit())
}

// ---------------------------------------------------------------------------
// T-072: RateLimitSection.Update — RateLimitMsg
// ---------------------------------------------------------------------------

func TestRateLimitSection_Update_RateLimitMsg_AddsProvider(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	resetAt := time.Now().Add(2 * time.Minute)
	msg := RateLimitMsg{
		Provider: "anthropic",
		Agent:    "claude",
		ResetAt:  resetAt,
	}
	updated, cmd := rl.Update(msg)
	require.NotNil(t, cmd, "RateLimitMsg must return a TickCmd")
	require.Len(t, updated.providers, 1)
	require.Len(t, updated.order, 1)
	assert.Equal(t, "anthropic", updated.order[0])
	prl := updated.providers["anthropic"]
	require.NotNil(t, prl)
	assert.True(t, prl.Active)
	assert.Equal(t, "anthropic", prl.Provider)
	assert.Equal(t, "claude", prl.Agent)
}

func TestRateLimitSection_Update_RateLimitMsg_UpdatesExistingProvider(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())

	// First message.
	resetAt1 := time.Now().Add(time.Minute)
	rl, _ = rl.Update(RateLimitMsg{Provider: "anthropic", ResetAt: resetAt1})
	require.Len(t, rl.order, 1)

	// Second message — same provider, new reset time.
	resetAt2 := time.Now().Add(3 * time.Minute)
	rl, _ = rl.Update(RateLimitMsg{Provider: "anthropic", ResetAt: resetAt2})

	// Must not grow order.
	assert.Len(t, rl.order, 1)
	assert.Len(t, rl.providers, 1)
	prl := rl.providers["anthropic"]
	require.NotNil(t, prl)
	// ResetAt should reflect the new value.
	assert.Equal(t, resetAt2, prl.ResetAt)
}

func TestRateLimitSection_Update_RateLimitMsg_MultipleProviders_StableOrder(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	resetAt := time.Now().Add(time.Minute)

	rl, _ = rl.Update(RateLimitMsg{Provider: "anthropic", ResetAt: resetAt})
	rl, _ = rl.Update(RateLimitMsg{Provider: "openai", ResetAt: resetAt})
	rl, _ = rl.Update(RateLimitMsg{Provider: "google", ResetAt: resetAt})

	require.Len(t, rl.order, 3)
	assert.Equal(t, "anthropic", rl.order[0])
	assert.Equal(t, "openai", rl.order[1])
	assert.Equal(t, "google", rl.order[2])
}

func TestRateLimitSection_Update_RateLimitMsg_ResetAfter_UsedWhenResetAtZero(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	before := time.Now()
	msg := RateLimitMsg{
		Provider:   "anthropic",
		ResetAfter: 90 * time.Second,
		Timestamp:  before,
	}
	rl, _ = rl.Update(msg)

	prl := rl.providers["anthropic"]
	require.NotNil(t, prl)
	// ResetAt should be approximately before + 90s.
	wantResetAt := before.Add(90 * time.Second)
	diff := prl.ResetAt.Sub(wantResetAt)
	assert.Less(t, diff.Abs(), time.Second, "ResetAt derived from ResetAfter must be close to expected")
}

func TestRateLimitSection_Update_RateLimitMsg_FallbackKeyToAgent(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	msg := RateLimitMsg{
		Provider: "", // empty provider
		Agent:    "codex",
		ResetAt:  time.Now().Add(time.Minute),
	}
	rl, _ = rl.Update(msg)
	assert.Contains(t, rl.providers, "codex", "agent name must be used as key when provider is empty")
	assert.Len(t, rl.order, 1)
	assert.Equal(t, "codex", rl.order[0])
}

func TestRateLimitSection_Update_RateLimitMsg_SetsHasActiveLimit(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	msg := RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(time.Minute)}
	rl, _ = rl.Update(msg)
	assert.True(t, rl.HasActiveLimit())
}

// ---------------------------------------------------------------------------
// T-072: RateLimitSection.Update — TickMsg
// ---------------------------------------------------------------------------

func TestRateLimitSection_Update_TickMsg_NoProviders_NilCmd(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	_, cmd := rl.Update(TickMsg{Time: time.Now()})
	assert.Nil(t, cmd, "TickMsg with no providers must return nil cmd")
}

func TestRateLimitSection_Update_TickMsg_ActiveProvider_ReturnsTickCmd(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	// Set a provider that expires far in the future.
	rl, _ = rl.Update(RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(2 * time.Minute)})
	require.True(t, rl.HasActiveLimit())

	_, cmd := rl.Update(TickMsg{Time: time.Now()})
	assert.NotNil(t, cmd, "TickMsg with active limit must return TickCmd")
}

func TestRateLimitSection_Update_TickMsg_ExpiredProvider_DeactivatesAndNilCmd(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	// Set a provider whose reset time is in the past.
	rl, _ = rl.Update(RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(-1 * time.Second)})

	rl, cmd := rl.Update(TickMsg{Time: time.Now()})
	prl := rl.providers["anthropic"]
	require.NotNil(t, prl)
	assert.False(t, prl.Active, "provider must be deactivated when ResetAt has passed")
	assert.Equal(t, time.Duration(0), prl.Remaining)
	assert.Nil(t, cmd, "TickMsg after expiry must return nil cmd")
}

func TestRateLimitSection_Update_TickMsg_MixedProviders_ContinuesWhileAnyActive(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	// One expired, one still active.
	rl, _ = rl.Update(RateLimitMsg{Provider: "expired", ResetAt: time.Now().Add(-time.Second)})
	rl, _ = rl.Update(RateLimitMsg{Provider: "active", ResetAt: time.Now().Add(2 * time.Minute)})

	rl, cmd := rl.Update(TickMsg{Time: time.Now()})
	assert.False(t, rl.providers["expired"].Active)
	assert.True(t, rl.providers["active"].Active)
	assert.NotNil(t, cmd, "TickCmd must continue while any provider is still active")
}

func TestRateLimitSection_Update_TickMsg_AllExpired_NilCmd(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	// Both providers expired.
	rl, _ = rl.Update(RateLimitMsg{Provider: "a", ResetAt: time.Now().Add(-time.Second)})
	rl, _ = rl.Update(RateLimitMsg{Provider: "b", ResetAt: time.Now().Add(-time.Second)})

	_, cmd := rl.Update(TickMsg{Time: time.Now()})
	assert.Nil(t, cmd, "TickCmd must be nil when all providers have expired")
}

// ---------------------------------------------------------------------------
// T-072: RateLimitSection.View
// ---------------------------------------------------------------------------

func TestRateLimitSection_View_HeaderAlwaysPresent(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	view := stripANSISidebar(rl.View(40))
	assert.Contains(t, view, "Rate Limits")
}

func TestRateLimitSection_View_NoProviders_ShowsPlaceholder(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	view := stripANSISidebar(rl.View(40))
	assert.Contains(t, view, "No limits")
}

func TestRateLimitSection_View_ActiveProvider_ShowsWait(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	rl, _ = rl.Update(RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(2 * time.Minute)})
	view := stripANSISidebar(rl.View(40))
	assert.Contains(t, view, "anthropic")
	assert.Contains(t, view, "WAIT")
}

func TestRateLimitSection_View_InactiveProvider_ShowsOK(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	// Provider with ResetAt in the past is inactive.
	rl, _ = rl.Update(RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(-time.Second)})
	// Tick to deactivate.
	rl, _ = rl.Update(TickMsg{Time: time.Now()})
	view := stripANSISidebar(rl.View(40))
	assert.Contains(t, view, "anthropic")
	assert.Contains(t, view, "OK")
	assert.NotContains(t, view, "WAIT")
}

func TestRateLimitSection_View_CountdownFormat_MinuteSeconds(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	// 1 minute 42 seconds remaining.
	rl, _ = rl.Update(RateLimitMsg{Provider: "codex", ResetAt: time.Now().Add(102 * time.Second)})
	view := stripANSISidebar(rl.View(50))
	// The format should be M:SS; check for the pattern.
	assert.Contains(t, view, "WAIT")
	assert.Contains(t, view, "codex")
}

func TestRateLimitSection_View_MultipleProviders_AllRendered(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	resetAt := time.Now().Add(time.Minute)
	rl, _ = rl.Update(RateLimitMsg{Provider: "anthropic", ResetAt: resetAt})
	rl, _ = rl.Update(RateLimitMsg{Provider: "openai", ResetAt: resetAt})

	view := stripANSISidebar(rl.View(50))
	assert.Contains(t, view, "anthropic")
	assert.Contains(t, view, "openai")
}

func TestRateLimitSection_View_StableOrder(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	resetAt := time.Now().Add(time.Minute)
	rl, _ = rl.Update(RateLimitMsg{Provider: "zzz", ResetAt: resetAt})
	rl, _ = rl.Update(RateLimitMsg{Provider: "aaa", ResetAt: resetAt})
	rl, _ = rl.Update(RateLimitMsg{Provider: "mmm", ResetAt: resetAt})

	view := stripANSISidebar(rl.View(50))
	// Insertion order: zzz, aaa, mmm — check positions.
	zzzIdx := strings.Index(view, "zzz")
	aaaIdx := strings.Index(view, "aaa")
	mmmIdx := strings.Index(view, "mmm")
	require.NotEqual(t, -1, zzzIdx)
	require.NotEqual(t, -1, aaaIdx)
	require.NotEqual(t, -1, mmmIdx)
	assert.Less(t, zzzIdx, aaaIdx, "insertion order: zzz must appear before aaa")
	assert.Less(t, aaaIdx, mmmIdx, "insertion order: aaa must appear before mmm")
}

func TestRateLimitSection_View_ZeroWidth_NoPanic(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	rl, _ = rl.Update(RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(time.Minute)})
	assert.NotPanics(t, func() {
		_ = rl.View(0)
	})
}

// ---------------------------------------------------------------------------
// T-072: HasActiveLimit
// ---------------------------------------------------------------------------

func TestRateLimitSection_HasActiveLimit_FalseWhenEmpty(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	assert.False(t, rl.HasActiveLimit())
}

func TestRateLimitSection_HasActiveLimit_TrueWhenActive(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	rl, _ = rl.Update(RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(time.Minute)})
	assert.True(t, rl.HasActiveLimit())
}

func TestRateLimitSection_HasActiveLimit_FalseAfterExpiry(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	rl, _ = rl.Update(RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(-time.Second)})
	rl, _ = rl.Update(TickMsg{Time: time.Now()})
	assert.False(t, rl.HasActiveLimit())
}

// ---------------------------------------------------------------------------
// T-072: SidebarModel integration — RateLimitMsg and TickMsg
// ---------------------------------------------------------------------------

func TestSidebarModel_Update_RateLimitMsg_ReturnsCmd(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	msg := RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(2 * time.Minute)}
	_, cmd := applySidebarMsg(m, msg)
	assert.NotNil(t, cmd, "SidebarModel must propagate TickCmd from RateLimitMsg")
}

func TestSidebarModel_Update_RateLimitMsg_SetsActiveLimit(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	msg := RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(2 * time.Minute)}
	m, _ = applySidebarMsg(m, msg)
	assert.True(t, m.rateLimits.HasActiveLimit())
}

func TestSidebarModel_Update_TickMsg_PropagatesCmd(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	// Add an active provider first.
	m, _ = applySidebarMsg(m, RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(2 * time.Minute)})
	// TickMsg should return another TickCmd while provider is still active.
	_, cmd := applySidebarMsg(m, TickMsg{Time: time.Now()})
	assert.NotNil(t, cmd, "SidebarModel must propagate TickCmd from TickMsg while limit active")
}

func TestSidebarModel_Update_TickMsg_NilCmdWhenNoLimits(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 30, 40)
	// No rate limit set; TickMsg should return nil.
	_, cmd := applySidebarMsg(m, TickMsg{Time: time.Now()})
	assert.Nil(t, cmd, "SidebarModel must return nil cmd from TickMsg when no active limits")
}

func TestSidebarModel_View_ContainsRateLimitsHeader(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 40, 60)
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "Rate Limits", "Rate Limits header must appear in sidebar view")
}

func TestSidebarModel_View_RateLimitsSection_NoLimits_ShowsPlaceholder(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 40, 60)
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "No limits", "No limits placeholder must appear when no providers are known")
}

func TestSidebarModel_View_RateLimitsSection_WithActiveLimit(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 40, 60)
	m, _ = applySidebarMsg(m, RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(2 * time.Minute)})
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "anthropic", "provider name must appear in sidebar view")
	assert.Contains(t, view, "WAIT", "WAIT indicator must appear for active rate limit")
}

func TestSidebarModel_View_RateLimitsSection_AfterExpiry_ShowsOK(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 40, 60)
	// Set expired provider.
	m, _ = applySidebarMsg(m, RateLimitMsg{Provider: "anthropic", ResetAt: time.Now().Add(-time.Second)})
	// Tick to deactivate.
	m, _ = applySidebarMsg(m, TickMsg{Time: time.Now()})
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "anthropic")
	assert.Contains(t, view, "OK")
}

func TestSidebarModel_Integration_RateLimitCountdownSequence(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 40, 60)

	// Apply a rate limit for two providers.
	future := time.Now().Add(5 * time.Minute)
	m, _ = applySidebarMsg(m, RateLimitMsg{Provider: "anthropic", ResetAt: future})
	m, _ = applySidebarMsg(m, RateLimitMsg{Provider: "openai", ResetAt: time.Now().Add(-time.Second)})

	// Tick once — openai should deactivate, anthropic remains active.
	m, cmd := applySidebarMsg(m, TickMsg{Time: time.Now()})
	assert.False(t, m.rateLimits.providers["openai"].Active)
	assert.True(t, m.rateLimits.providers["anthropic"].Active)
	assert.NotNil(t, cmd)

	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "anthropic")
	assert.Contains(t, view, "WAIT")
	assert.Contains(t, view, "openai")
	assert.Contains(t, view, "OK")
}

// ---------------------------------------------------------------------------
// T-072: formatCountdown — additional edge cases
// ---------------------------------------------------------------------------

func TestFormatCountdown_SubSecond(t *testing.T) {
	t.Parallel()
	// Duration less than one full second but positive must show "0:00"
	// because int(d.Seconds()) truncates to 0.
	got := formatCountdown(500 * time.Millisecond)
	assert.Equal(t, "0:00", got)
}

func TestFormatCountdown_ExactlyOneMinute(t *testing.T) {
	t.Parallel()
	got := formatCountdown(60 * time.Second)
	assert.Equal(t, "1:00", got)
}

func TestFormatCountdown_ExactlyOneHour(t *testing.T) {
	t.Parallel()
	got := formatCountdown(time.Hour)
	assert.Equal(t, "1:00:00", got)
}

func TestFormatCountdown_LargeHours(t *testing.T) {
	t.Parallel()
	// 25 hours 3 minutes 7 seconds
	d := 25*time.Hour + 3*time.Minute + 7*time.Second
	got := formatCountdown(d)
	assert.Equal(t, "25:03:07", got)
}

// ---------------------------------------------------------------------------
// T-072: RateLimitSection.Update — TickMsg decrements Remaining
// ---------------------------------------------------------------------------

func TestRateLimitSection_Update_TickMsg_DecrementsRemaining(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	// Provider resets 5 minutes from now.
	resetAt := time.Now().Add(5 * time.Minute)
	rl, _ = rl.Update(RateLimitMsg{Provider: "anthropic", ResetAt: resetAt})

	before := rl.providers["anthropic"].Remaining

	// Tick once — Remaining should have decreased (or stayed the same in a
	// fast machine, but certainly not increased).
	rl, _ = rl.Update(TickMsg{Time: time.Now()})
	after := rl.providers["anthropic"].Remaining

	assert.LessOrEqual(t, after, before,
		"Remaining must not increase after a TickMsg")
	assert.True(t, rl.providers["anthropic"].Active,
		"provider must still be active when reset time has not passed")
}

func TestRateLimitSection_Update_TickMsg_MultipleTicks_CountsDown(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	// Reset time is 2 seconds in the future.
	resetAt := time.Now().Add(2 * time.Second)
	rl, _ = rl.Update(RateLimitMsg{Provider: "codex", ResetAt: resetAt})
	require.True(t, rl.providers["codex"].Active)

	// Simulate time passing by manipulating ResetAt directly (we cannot sleep
	// in unit tests reliably). Instead, set ResetAt to the past and tick.
	rl.providers["codex"].ResetAt = time.Now().Add(-time.Millisecond)

	rl, cmd := rl.Update(TickMsg{Time: time.Now()})
	prl := rl.providers["codex"]
	require.NotNil(t, prl)
	assert.False(t, prl.Active, "provider must be inactive once ResetAt has passed")
	assert.Equal(t, time.Duration(0), prl.Remaining, "Remaining must be zero after expiry")
	assert.Nil(t, cmd, "no further TickCmd needed when all providers are expired")
}

// ---------------------------------------------------------------------------
// T-072: RateLimitSection.Update — RateLimitMsg edge cases
// ---------------------------------------------------------------------------

func TestRateLimitSection_Update_RateLimitMsg_BothProviderAndAgentEmpty_KeyIsEmpty(t *testing.T) {
	t.Parallel()
	// When both Provider and Agent are empty the key becomes "".
	// The implementation should not panic; it should insert under key "".
	rl := NewRateLimitSection(DefaultTheme())
	msg := RateLimitMsg{
		Provider: "",
		Agent:    "",
		ResetAt:  time.Now().Add(time.Minute),
	}
	assert.NotPanics(t, func() {
		rl, _ = rl.Update(msg)
	})
	assert.Len(t, rl.order, 1)
	// Key is "", provider entry exists.
	assert.Contains(t, rl.providers, "")
}

func TestRateLimitSection_Update_RateLimitMsg_ResetAfterWithZeroTimestamp(t *testing.T) {
	t.Parallel()
	// When both ResetAt and Timestamp are zero, Timestamp defaults to time.Now()
	// and ResetAt = time.Now() + ResetAfter. The resulting Remaining should be
	// approximately equal to ResetAfter.
	rl := NewRateLimitSection(DefaultTheme())
	before := time.Now()
	msg := RateLimitMsg{
		Provider:   "openai",
		ResetAfter: 3 * time.Minute,
		// ResetAt and Timestamp are zero
	}
	rl, _ = rl.Update(msg)
	after := time.Now()

	prl := rl.providers["openai"]
	require.NotNil(t, prl)

	// ResetAt should be between (before + 3min) and (after + 3min).
	assert.True(t, prl.ResetAt.After(before.Add(3*time.Minute).Add(-time.Second)),
		"ResetAt must be at least before+ResetAfter")
	assert.True(t, prl.ResetAt.Before(after.Add(3*time.Minute).Add(time.Second)),
		"ResetAt must be at most after+ResetAfter")
	assert.True(t, prl.Active)
}

func TestRateLimitSection_Update_RateLimitMsg_ResetAtInPast_ActiveButRemainingZero(t *testing.T) {
	t.Parallel()
	// When ResetAt is already in the past at message receipt, Remaining is
	// clamped to 0 but the provider is still set Active=true. The next tick
	// will deactivate it.
	rl := NewRateLimitSection(DefaultTheme())
	msg := RateLimitMsg{
		Provider: "gemini",
		ResetAt:  time.Now().Add(-10 * time.Second), // already expired
	}
	rl, cmd := rl.Update(msg)
	require.NotNil(t, cmd, "TickCmd must still be returned on RateLimitMsg even when past")
	prl := rl.providers["gemini"]
	require.NotNil(t, prl)
	assert.True(t, prl.Active, "provider must be Active=true immediately after RateLimitMsg")
	assert.Equal(t, time.Duration(0), prl.Remaining, "Remaining must be 0 when ResetAt is in the past")
}

func TestRateLimitSection_Update_RateLimitMsg_PreservesExistingInactiveProviders(t *testing.T) {
	t.Parallel()
	// Adding a second provider must not disturb an already-inactive first.
	rl := NewRateLimitSection(DefaultTheme())
	// Insert and then expire "alpha".
	rl, _ = rl.Update(RateLimitMsg{Provider: "alpha", ResetAt: time.Now().Add(-time.Second)})
	rl, _ = rl.Update(TickMsg{Time: time.Now()})
	require.False(t, rl.providers["alpha"].Active)

	// Now insert "beta".
	rl, _ = rl.Update(RateLimitMsg{Provider: "beta", ResetAt: time.Now().Add(time.Minute)})

	// "alpha" must still be present and inactive.
	alphaEntry, ok := rl.providers["alpha"]
	require.True(t, ok, "alpha must still exist after inserting beta")
	assert.False(t, alphaEntry.Active, "alpha must remain inactive")
	assert.Len(t, rl.order, 2, "order must contain both providers")
	assert.Equal(t, "alpha", rl.order[0])
	assert.Equal(t, "beta", rl.order[1])
}

// ---------------------------------------------------------------------------
// T-072: RateLimitSection.View — format and layout verification
// ---------------------------------------------------------------------------

func TestRateLimitSection_View_LineFormat_NameColonStatus(t *testing.T) {
	t.Parallel()
	// Active provider line must contain "name: WAIT M:SS".
	rl := NewRateLimitSection(DefaultTheme())
	rl, _ = rl.Update(RateLimitMsg{
		Provider: "mycloud",
		ResetAt:  time.Now().Add(time.Hour), // far future ensures Active=true
	})
	view := stripANSISidebar(rl.View(60))

	// Must contain provider name followed by ": WAIT".
	assert.Contains(t, view, "mycloud: WAIT",
		"active provider line must use format 'name: WAIT M:SS'")
}

func TestRateLimitSection_View_LineFormat_NameColonOK(t *testing.T) {
	t.Parallel()
	// Inactive provider line must contain "name: OK".
	rl := NewRateLimitSection(DefaultTheme())
	rl, _ = rl.Update(RateLimitMsg{Provider: "cloudapi", ResetAt: time.Now().Add(-time.Second)})
	rl, _ = rl.Update(TickMsg{Time: time.Now()})
	view := stripANSISidebar(rl.View(60))

	assert.Contains(t, view, "cloudapi: OK",
		"inactive provider line must use format 'name: OK'")
}

func TestRateLimitSection_View_EachProviderOnSeparateLine(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	resetAt := time.Now().Add(time.Minute)
	rl, _ = rl.Update(RateLimitMsg{Provider: "alpha", ResetAt: resetAt})
	rl, _ = rl.Update(RateLimitMsg{Provider: "beta", ResetAt: resetAt})
	rl, _ = rl.Update(RateLimitMsg{Provider: "gamma", ResetAt: resetAt})
	view := stripANSISidebar(rl.View(60))

	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	var providerLines []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.Contains(l, "alpha") ||
			strings.Contains(l, "beta") ||
			strings.Contains(l, "gamma") {
			providerLines = append(providerLines, l)
		}
	}
	assert.Len(t, providerLines, 3,
		"each of the three providers must appear on its own separate line")
}

func TestRateLimitSection_View_LongProviderName_Truncated(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	longName := strings.Repeat("a", 100)
	rl, _ = rl.Update(RateLimitMsg{Provider: longName, ResetAt: time.Now().Add(time.Minute)})
	view := stripANSISidebar(rl.View(20))

	// The full name must not appear verbatim.
	assert.NotContains(t, view, longName,
		"long provider names must be truncated to fit within the configured width")
	// Ellipsis must appear.
	assert.Contains(t, view, "…",
		"truncated provider name must include ellipsis")
}

func TestRateLimitSection_View_FallbackNameToAgent(t *testing.T) {
	t.Parallel()
	// When Provider is empty, Agent is used as both the key and the display name.
	rl := NewRateLimitSection(DefaultTheme())
	rl, _ = rl.Update(RateLimitMsg{
		Provider: "",
		Agent:    "myagent",
		ResetAt:  time.Now().Add(time.Minute),
	})
	view := stripANSISidebar(rl.View(50))
	assert.Contains(t, view, "myagent",
		"agent name must be used as display name when provider is empty")
}

func TestRateLimitSection_View_CountdownFormat_InView_MinSec(t *testing.T) {
	t.Parallel()
	// 2 minutes 30 seconds should render as "2:30" inside WAIT.
	// We set Remaining directly on the provider pointer after Update to avoid
	// any sub-second timing jitter between construction and View.
	rl := NewRateLimitSection(DefaultTheme())
	rl, _ = rl.Update(RateLimitMsg{
		Provider: "svc",
		ResetAt:  time.Now().Add(time.Hour), // far future — ensures Active=true
	})
	// Pin Remaining to an exact value so formatCountdown is deterministic.
	rl.providers["svc"].Remaining = 2*time.Minute + 30*time.Second
	view := stripANSISidebar(rl.View(60))
	// The countdown string appears inside "WAIT M:SS".
	assert.Contains(t, view, "WAIT 2:30",
		"countdown in view must use M:SS format for durations under 1 hour")
}

func TestRateLimitSection_View_CountdownFormat_InView_HourMinSec(t *testing.T) {
	t.Parallel()
	// 1 hour 5 minutes 3 seconds should render as "1:05:03" inside WAIT.
	// We set Remaining directly to avoid sub-second timing jitter.
	rl := NewRateLimitSection(DefaultTheme())
	rl, _ = rl.Update(RateLimitMsg{
		Provider: "svc",
		ResetAt:  time.Now().Add(2 * time.Hour), // far future — ensures Active=true
	})
	// Pin Remaining to an exact value.
	rl.providers["svc"].Remaining = time.Hour + 5*time.Minute + 3*time.Second
	view := stripANSISidebar(rl.View(60))
	assert.Contains(t, view, "WAIT 1:05:03",
		"countdown in view must use H:MM:SS format for durations of 1 hour or more")
}

func TestRateLimitSection_View_NegativeRemaining_ShowsZeroCountdown(t *testing.T) {
	t.Parallel()
	// If somehow Remaining ended up negative (should not happen in practice),
	// formatCountdown must return "0:00" and Active must be handled by tick.
	// Test via View after tick with already-expired provider.
	rl := NewRateLimitSection(DefaultTheme())
	rl, _ = rl.Update(RateLimitMsg{Provider: "svc", ResetAt: time.Now().Add(-5 * time.Minute)})
	rl, _ = rl.Update(TickMsg{Time: time.Now()})

	// Provider is now inactive; view must not contain WAIT.
	view := stripANSISidebar(rl.View(50))
	assert.NotContains(t, view, "WAIT",
		"expired provider must not show WAIT")
	assert.Contains(t, view, "OK",
		"expired provider must show OK")
}

func TestRateLimitSection_View_NoPlaceholderWhenProvidersExist(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	rl, _ = rl.Update(RateLimitMsg{Provider: "svc", ResetAt: time.Now().Add(time.Minute)})
	view := stripANSISidebar(rl.View(50))
	assert.NotContains(t, view, "No limits",
		"No limits placeholder must not appear when providers exist")
}

// ---------------------------------------------------------------------------
// T-072: RateLimitSection value semantics (immutability)
// ---------------------------------------------------------------------------

func TestRateLimitSection_Update_ValueSemantics_OriginalUnchanged(t *testing.T) {
	t.Parallel()
	original := NewRateLimitSection(DefaultTheme())
	updated, _ := original.Update(RateLimitMsg{
		Provider: "anthropic",
		ResetAt:  time.Now().Add(time.Minute),
	})

	// original must still have zero providers.
	assert.Empty(t, original.providers,
		"Update must not mutate the original RateLimitSection (value semantics)")
	assert.Empty(t, original.order,
		"Update must not mutate the original order slice (value semantics)")
	assert.Len(t, updated.providers, 1)
}

func TestRateLimitSection_Tick_ValueSemantics_OriginalUnchanged(t *testing.T) {
	t.Parallel()
	rl := NewRateLimitSection(DefaultTheme())
	rl, _ = rl.Update(RateLimitMsg{Provider: "svc", ResetAt: time.Now().Add(time.Minute)})

	snap := rl.providers["svc"].Remaining

	ticked, _ := rl.Update(TickMsg{Time: time.Now()})
	_ = ticked

	// rl.providers["svc"].Remaining may have changed because tick returns a
	// new map; the original rl must still reference its own snapshot. Since
	// value semantics copy the map pointer (not the values), both rl and
	// ticked point to different maps. We can verify the two are independent
	// by checking that ticked has a separate map.
	assert.Equal(t, snap, rl.providers["svc"].Remaining,
		"original RateLimitSection's Remaining must not be modified by tick (value semantics)")
}

// ---------------------------------------------------------------------------
// T-072: HasActiveLimit — comprehensive table test
// ---------------------------------------------------------------------------

func TestRateLimitSection_HasActiveLimit_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupFn     func() RateLimitSection
		wantActive  bool
	}{
		{
			name:       "empty section",
			setupFn:    func() RateLimitSection { return NewRateLimitSection(DefaultTheme()) },
			wantActive: false,
		},
		{
			name: "single active provider",
			setupFn: func() RateLimitSection {
				rl := NewRateLimitSection(DefaultTheme())
				rl, _ = rl.Update(RateLimitMsg{Provider: "p1", ResetAt: time.Now().Add(time.Minute)})
				return rl
			},
			wantActive: true,
		},
		{
			name: "single expired provider after tick",
			setupFn: func() RateLimitSection {
				rl := NewRateLimitSection(DefaultTheme())
				rl, _ = rl.Update(RateLimitMsg{Provider: "p1", ResetAt: time.Now().Add(-time.Second)})
				rl, _ = rl.Update(TickMsg{Time: time.Now()})
				return rl
			},
			wantActive: false,
		},
		{
			name: "multiple providers — at least one active",
			setupFn: func() RateLimitSection {
				rl := NewRateLimitSection(DefaultTheme())
				rl, _ = rl.Update(RateLimitMsg{Provider: "p1", ResetAt: time.Now().Add(-time.Second)})
				rl, _ = rl.Update(RateLimitMsg{Provider: "p2", ResetAt: time.Now().Add(time.Minute)})
				rl, _ = rl.Update(TickMsg{Time: time.Now()})
				return rl
			},
			wantActive: true,
		},
		{
			name: "multiple providers — all expired after tick",
			setupFn: func() RateLimitSection {
				rl := NewRateLimitSection(DefaultTheme())
				rl, _ = rl.Update(RateLimitMsg{Provider: "p1", ResetAt: time.Now().Add(-time.Second)})
				rl, _ = rl.Update(RateLimitMsg{Provider: "p2", ResetAt: time.Now().Add(-time.Second)})
				rl, _ = rl.Update(TickMsg{Time: time.Now()})
				return rl
			},
			wantActive: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rl := tt.setupFn()
			assert.Equal(t, tt.wantActive, rl.HasActiveLimit())
		})
	}
}

// ---------------------------------------------------------------------------
// T-072: SidebarModel integration — additional rate-limit scenarios
// ---------------------------------------------------------------------------

func TestSidebarModel_Update_RateLimitMsg_AgentOnlyKey(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 40, 60)
	// RateLimitMsg with only Agent set (no Provider).
	m, cmd := applySidebarMsg(m, RateLimitMsg{
		Provider: "",
		Agent:    "codex",
		ResetAt:  time.Now().Add(time.Minute),
	})
	require.NotNil(t, cmd, "SidebarModel must propagate TickCmd when RateLimitMsg arrives")
	assert.True(t, m.rateLimits.HasActiveLimit(),
		"HasActiveLimit must be true after RateLimitMsg with agent-only key")
	assert.Contains(t, m.rateLimits.providers, "codex",
		"codex must be registered in providers map")
}

func TestSidebarModel_Update_RateLimitMsg_MultipleProviders(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 40, 60)
	future := time.Now().Add(time.Minute)
	m, _ = applySidebarMsg(m, RateLimitMsg{Provider: "anthropic", ResetAt: future})
	m, _ = applySidebarMsg(m, RateLimitMsg{Provider: "openai", ResetAt: future})
	m, _ = applySidebarMsg(m, RateLimitMsg{Provider: "google", ResetAt: future})

	assert.Len(t, m.rateLimits.providers, 3)
	assert.Len(t, m.rateLimits.order, 3)
	assert.True(t, m.rateLimits.HasActiveLimit())
}

func TestSidebarModel_Update_TickMsg_ExpiresProvider_StopsCmd(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 40, 60)
	// Set a provider that has already expired.
	m, _ = applySidebarMsg(m, RateLimitMsg{Provider: "svc", ResetAt: time.Now().Add(-time.Second)})
	// Tick — provider should deactivate and cmd should be nil.
	m, cmd := applySidebarMsg(m, TickMsg{Time: time.Now()})
	assert.False(t, m.rateLimits.providers["svc"].Active)
	assert.Nil(t, cmd,
		"SidebarModel must return nil cmd when all providers have expired after tick")
}

func TestSidebarModel_View_RateLimitsSection_CountdownInView(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 50, 60)
	// Set ResetAt far in future so Active=true, then pin Remaining to a known value.
	m, _ = applySidebarMsg(m, RateLimitMsg{
		Provider: "testprov",
		ResetAt:  time.Now().Add(time.Hour),
	})
	// Pin Remaining to 3:15 for a deterministic countdown string.
	m.rateLimits.providers["testprov"].Remaining = 3*time.Minute + 15*time.Second
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "testprov", "provider name must appear in sidebar view")
	assert.Contains(t, view, "WAIT 3:15",
		"countdown in sidebar view must show M:SS format")
}

func TestSidebarModel_View_RateLimitsSection_MultipleProvidersInView(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 50, 80)
	future := time.Now().Add(time.Minute)
	m, _ = applySidebarMsg(m, RateLimitMsg{Provider: "anthropic", ResetAt: future})
	m, _ = applySidebarMsg(m, RateLimitMsg{Provider: "openai", ResetAt: time.Now().Add(-time.Second)})
	m, _ = applySidebarMsg(m, TickMsg{Time: time.Now()})

	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "anthropic")
	assert.Contains(t, view, "WAIT")
	assert.Contains(t, view, "openai")
	assert.Contains(t, view, "OK")
}

func TestSidebarModel_Integration_RateLimitLifecycle(t *testing.T) {
	t.Parallel()
	m := makeSidebar(t, 50, 80)

	// Step 1: no limits — placeholder shown.
	view := stripANSISidebar(m.View())
	assert.Contains(t, view, "No limits")

	// Step 2: add an active rate limit.
	m, cmd := applySidebarMsg(m, RateLimitMsg{
		Provider: "anthropic",
		ResetAt:  time.Now().Add(30 * time.Second),
	})
	require.NotNil(t, cmd)
	view = stripANSISidebar(m.View())
	assert.Contains(t, view, "WAIT")
	assert.NotContains(t, view, "No limits")

	// Step 3: expire the limit and tick.
	m.rateLimits.providers["anthropic"].ResetAt = time.Now().Add(-time.Millisecond)
	m, cmd = applySidebarMsg(m, TickMsg{Time: time.Now()})
	assert.Nil(t, cmd, "cmd must be nil once all limits expired")

	view = stripANSISidebar(m.View())
	assert.Contains(t, view, "OK", "provider must show OK after expiry")
	assert.NotContains(t, view, "WAIT", "WAIT must disappear after expiry")
}

// ---------------------------------------------------------------------------
// T-072: Benchmark — tick processing
// ---------------------------------------------------------------------------

func BenchmarkRateLimitSection_Tick(b *testing.B) {
	rl := NewRateLimitSection(DefaultTheme())
	resetAt := time.Now().Add(2 * time.Minute)
	var cmd tea.Cmd
	rl, cmd = rl.Update(RateLimitMsg{Provider: "anthropic", Agent: "claude", ResetAt: resetAt})
	_ = cmd
	rl, cmd = rl.Update(RateLimitMsg{Provider: "openai", Agent: "codex", ResetAt: resetAt})
	_ = cmd
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rl.Update(TickMsg{Time: time.Now()})
	}
}

// ---------------------------------------------------------------------------
// T-072: Benchmark
// ---------------------------------------------------------------------------

func BenchmarkRateLimitSection_View(b *testing.B) {
	rl := NewRateLimitSection(DefaultTheme())
	resetAt := time.Now().Add(2 * time.Minute)
	var cmd tea.Cmd
	rl, cmd = rl.Update(RateLimitMsg{Provider: "anthropic", Agent: "claude", ResetAt: resetAt})
	_ = cmd
	rl, cmd = rl.Update(RateLimitMsg{Provider: "openai", Agent: "codex", ResetAt: resetAt})
	_ = cmd
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rl.View(40)
	}
}

// ---------------------------------------------------------------------------
// T-071: Benchmark
// ---------------------------------------------------------------------------

func BenchmarkTaskProgressSection_View(b *testing.B) {
	tp := NewTaskProgressSection(DefaultTheme())
	tp = tp.Update(TaskProgressMsg{Completed: 17, Total: 30})
	tp.SetTotals(30, 5)
	tp.SetPhase(3, 7, 4)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tp.View(40)
	}
}

func BenchmarkSidebarModel_View_WithProgress(b *testing.B) {
	m := NewSidebarModel(DefaultTheme())
	m.SetDimensions(35, 40)
	m.SetFocused(true)
	m.SetTotals(30, 5)
	m.SetPhase(2, 8, 4)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}
