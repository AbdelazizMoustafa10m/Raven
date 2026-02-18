package tui

import (
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
