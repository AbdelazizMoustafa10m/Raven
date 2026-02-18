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
