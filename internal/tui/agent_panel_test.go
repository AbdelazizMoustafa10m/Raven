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
// Helpers
// ---------------------------------------------------------------------------

// stripANSIPanel removes ANSI escape sequences from a string so tests can
// inspect raw text content without terminal colour codes.
func stripANSIPanel(s string) string {
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

// makePanel is a convenience constructor that creates a dimensioned, focused
// AgentPanelModel for use in tests.
func makePanel(t *testing.T, width, height int) AgentPanelModel {
	t.Helper()
	m := NewAgentPanelModel(DefaultTheme())
	m.SetDimensions(width, height)
	m.SetFocused(true)
	return m
}

// sendOutput dispatches an AgentOutputMsg to the panel and returns the updated
// model.
func sendOutput(ap AgentPanelModel, agent, line string) AgentPanelModel {
	updated, _ := ap.Update(AgentOutputMsg{
		Agent:     agent,
		Line:      line,
		Stream:    "stdout",
		Timestamp: time.Now(),
	})
	return updated
}

// sendStatus dispatches an AgentStatusMsg to the panel and returns the updated
// model.
func sendStatus(ap AgentPanelModel, agent string, status AgentStatus, task, detail string) AgentPanelModel {
	updated, _ := ap.Update(AgentStatusMsg{
		Agent:     agent,
		Status:    status,
		Task:      task,
		Detail:    detail,
		Timestamp: time.Now(),
	})
	return updated
}

// pressKey dispatches a tea.KeyMsg to the panel and returns the updated model
// and any command.
func pressKey(ap AgentPanelModel, keyType tea.KeyType) (AgentPanelModel, tea.Cmd) {
	return ap.Update(tea.KeyMsg{Type: keyType})
}

// ---------------------------------------------------------------------------
// OutputBuffer — unit tests
// ---------------------------------------------------------------------------

// TestOutputBuffer_AppendFewLines verifies that appending fewer lines than the
// capacity returns all lines in insertion order.
func TestOutputBuffer_AppendFewLines(t *testing.T) {
	t.Parallel()

	b := NewOutputBuffer(5)
	b.Append("line1")
	b.Append("line2")
	b.Append("line3")

	lines := b.Lines()
	require.Len(t, lines, 3)
	assert.Equal(t, "line1", lines[0])
	assert.Equal(t, "line2", lines[1])
	assert.Equal(t, "line3", lines[2])
}

// TestOutputBuffer_EvictOnOverflow verifies that when capacity is exceeded the
// oldest lines are evicted and only the most recent capacity lines remain.
func TestOutputBuffer_EvictOnOverflow(t *testing.T) {
	t.Parallel()

	b := NewOutputBuffer(5)
	for i := 1; i <= 7; i++ {
		b.Append(fmt.Sprintf("line%d", i))
	}

	lines := b.Lines()
	require.Len(t, lines, 5, "buffer must retain exactly capacity lines after overflow")
	assert.Equal(t, "line3", lines[0], "oldest retained line should be line3")
	assert.Equal(t, "line4", lines[1])
	assert.Equal(t, "line5", lines[2])
	assert.Equal(t, "line6", lines[3])
	assert.Equal(t, "line7", lines[4], "newest line should be line7")
}

// TestOutputBuffer_Len verifies Len() before and after an eviction.
func TestOutputBuffer_Len(t *testing.T) {
	t.Parallel()

	b := NewOutputBuffer(5)

	// Fill to capacity.
	for i := 0; i < 5; i++ {
		assert.Equal(t, i, b.Len(), "Len must equal number of appended lines before capacity")
		b.Append(fmt.Sprintf("line%d", i))
	}
	assert.Equal(t, 5, b.Len(), "Len must equal capacity after filling")

	// Overflow: count must stay pinned at capacity.
	b.Append("overflow")
	assert.Equal(t, 5, b.Len(), "Len must not exceed capacity after overflow")

	b.Append("overflow2")
	assert.Equal(t, 5, b.Len(), "Len must remain at capacity after multiple overflows")
}

// ---------------------------------------------------------------------------
// AgentPanelModel construction
// ---------------------------------------------------------------------------

// TestNewAgentPanelModel_Empty verifies that a freshly constructed panel has
// no agents and ActiveAgent returns an empty string.
func TestNewAgentPanelModel_Empty(t *testing.T) {
	t.Parallel()

	ap := NewAgentPanelModel(DefaultTheme())

	assert.Equal(t, "", ap.ActiveAgent(), "ActiveAgent must be empty when no agents registered")
	assert.Empty(t, ap.agentOrder, "agentOrder must be empty initially")
}

// ---------------------------------------------------------------------------
// AgentPanelModel.Update — AgentOutputMsg
// ---------------------------------------------------------------------------

// TestUpdate_AgentOutputMsg_NewLine verifies that sending an AgentOutputMsg
// for a known agent appends the line to that agent's buffer.
func TestUpdate_AgentOutputMsg_NewLine(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", "hello")

	av, ok := ap.agents["claude"]
	require.True(t, ok, "agent claude must exist after receiving output")
	lines := av.buffer.Lines()
	require.Len(t, lines, 1)
	assert.Equal(t, "hello", lines[0])
}

// TestUpdate_AgentOutputMsg_CreatesNewAgent verifies that an AgentOutputMsg
// for an unknown agent creates a new AgentView.
func TestUpdate_AgentOutputMsg_CreatesNewAgent(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)

	// No agents yet.
	require.Empty(t, ap.agentOrder)

	ap = sendOutput(ap, "codex", "first output")

	assert.Contains(t, ap.agentOrder, "codex", "codex must appear in agentOrder after first output")
	_, ok := ap.agents["codex"]
	assert.True(t, ok, "agents map must contain codex")
}

// ---------------------------------------------------------------------------
// AgentPanelModel.Update — AgentStatusMsg
// ---------------------------------------------------------------------------

// TestUpdate_AgentStatusMsg_UpdatesStatus verifies that an AgentStatusMsg
// updates the agent's status, task, and detail fields.
func TestUpdate_AgentStatusMsg_UpdatesStatus(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendStatus(ap, "claude", AgentRunning, "T-001", "implementing feature")

	av, ok := ap.agents["claude"]
	require.True(t, ok, "agent claude must exist after status message")
	assert.Equal(t, AgentRunning, av.status, "status must be AgentRunning")
	assert.Equal(t, "T-001", av.task, "task must be T-001")
	assert.Equal(t, "implementing feature", av.detail, "detail must match")
}

// ---------------------------------------------------------------------------
// AgentPanelModel.Update — keyboard / tab switching
// ---------------------------------------------------------------------------

// TestUpdate_KeyTab_SwitchesTab verifies that pressing Tab when two agents are
// present and panel is focused advances activeTab from 0 to 1.
func TestUpdate_KeyTab_SwitchesTab(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", "line1")
	ap = sendOutput(ap, "codex", "line2")

	require.Equal(t, 0, ap.activeTab, "activeTab must start at 0")
	require.Equal(t, "claude", ap.ActiveAgent())

	ap, cmd := pressKey(ap, tea.KeyTab)
	assert.Nil(t, cmd, "no cmd expected when tab switches between 2+ agents")
	assert.Equal(t, 1, ap.activeTab, "activeTab must advance to 1 after Tab")
	assert.Equal(t, "codex", ap.ActiveAgent(), "active agent must be codex")
}

// TestUpdate_KeyShiftTab_SwitchesTabBackwards verifies that Shift+Tab cycles
// backwards through agent tabs.
func TestUpdate_KeyShiftTab_SwitchesTabBackwards(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", "line1")
	ap = sendOutput(ap, "codex", "line2")

	// Start at tab 1 (codex).
	ap.activeTab = 1

	ap, cmd := pressKey(ap, tea.KeyShiftTab)
	assert.Nil(t, cmd, "no cmd expected when shift-tab switches between 2+ agents")
	assert.Equal(t, 0, ap.activeTab, "activeTab must retreat to 0 after ShiftTab")
	assert.Equal(t, "claude", ap.ActiveAgent())
}

// TestUpdate_KeyTab_WrapAround verifies that Tab wraps around from the last
// tab to the first.
func TestUpdate_KeyTab_WrapAround(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", "line1")
	ap = sendOutput(ap, "codex", "line2")

	// Move to last tab.
	ap.activeTab = 1

	ap, cmd := pressKey(ap, tea.KeyTab)
	assert.Nil(t, cmd)
	assert.Equal(t, 0, ap.activeTab, "Tab from last tab must wrap to 0")
	assert.Equal(t, "claude", ap.ActiveAgent())
}

// TestUpdate_KeyShiftTab_WrapAround verifies that Shift+Tab wraps backwards
// from the first tab to the last.
func TestUpdate_KeyShiftTab_WrapAround(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", "line1")
	ap = sendOutput(ap, "codex", "line2")

	// Start at first tab.
	ap.activeTab = 0

	ap, cmd := pressKey(ap, tea.KeyShiftTab)
	assert.Nil(t, cmd)
	assert.Equal(t, 1, ap.activeTab, "ShiftTab from first tab must wrap to last")
	assert.Equal(t, "codex", ap.ActiveAgent())
}

// ---------------------------------------------------------------------------
// ActiveAgent
// ---------------------------------------------------------------------------

// TestActiveAgent_ReturnsCorrectName verifies that ActiveAgent returns the
// name of the currently active tab after multiple agents are added.
func TestActiveAgent_ReturnsCorrectName(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", "a")
	ap = sendOutput(ap, "codex", "b")
	ap = sendOutput(ap, "gemini", "c")

	// Default tab is 0 → claude.
	assert.Equal(t, "claude", ap.ActiveAgent())

	// Switch to tab 2 → gemini.
	ap.activeTab = 2
	assert.Equal(t, "gemini", ap.ActiveAgent())

	// Switch to tab 1 → codex.
	ap.activeTab = 1
	assert.Equal(t, "codex", ap.ActiveAgent())
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// TestView_NoAgents_ShowsPlaceholder verifies that View() renders a
// placeholder when no agents have been registered.
func TestView_NoAgents_ShowsPlaceholder(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	output := stripANSIPanel(ap.View())

	assert.Contains(t, output, "Waiting for agents...", "placeholder must be visible when no agents exist")
}

// TestView_OneAgent_NoTabBar verifies that with a single agent the output
// contains the agent header but no tab bar (no second agent name in tabs).
func TestView_OneAgent_NoTabBar(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", "hello from claude")

	output := stripANSIPanel(ap.View())

	// Agent header/name must appear.
	assert.Contains(t, output, "claude", "agent name must appear in header")

	// With a single agent there should be no tab bar; the panel must NOT
	// render a second row of tabs (i.e. "codex" must not be present).
	assert.NotContains(t, output, "codex", "no tab bar should exist for a single agent")
}

// TestView_TwoAgents_ShowsTabBar verifies that with two agents the tab bar
// renders both agent names.
func TestView_TwoAgents_ShowsTabBar(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", "output from claude")
	ap = sendOutput(ap, "codex", "output from codex")

	output := stripANSIPanel(ap.View())

	assert.Contains(t, output, "claude", "tab bar must show claude")
	assert.Contains(t, output, "codex", "tab bar must show codex")
}

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

// TestIntegration_RapidOutput_BufferCap verifies that feeding 1500 lines into
// the buffer never causes it to exceed MaxOutputLines (1000).
func TestIntegration_RapidOutput_BufferCap(t *testing.T) {
	t.Parallel()

	const totalLines = 1500

	ap := makePanel(t, 80, 40)
	for i := 0; i < totalLines; i++ {
		ap = sendOutput(ap, "claude", fmt.Sprintf("line %d", i))
	}

	av, ok := ap.agents["claude"]
	require.True(t, ok)
	assert.LessOrEqual(t, av.buffer.Len(), MaxOutputLines,
		"buffer must never exceed MaxOutputLines after %d appends", totalLines)
	assert.Equal(t, MaxOutputLines, av.buffer.Len(),
		"buffer must be exactly MaxOutputLines after overflow")

	// The last line in the buffer must be the most recently appended one.
	lines := av.buffer.Lines()
	require.NotEmpty(t, lines)
	assert.Equal(t, fmt.Sprintf("line %d", totalLines-1), lines[len(lines)-1],
		"last buffered line must be the final appended line")
}

// TestIntegration_MultipleAgents_Interleaved verifies that interleaved output
// from two agents is correctly routed to each agent's own buffer.
func TestIntegration_MultipleAgents_Interleaved(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 40)

	for i := 0; i < 10; i++ {
		ap = sendOutput(ap, "claude", fmt.Sprintf("claude line %d", i))
		ap = sendOutput(ap, "codex", fmt.Sprintf("codex line %d", i))
	}

	claudeView, ok := ap.agents["claude"]
	require.True(t, ok, "claude agent view must exist")
	codexView, ok := ap.agents["codex"]
	require.True(t, ok, "codex agent view must exist")

	assert.Equal(t, 10, claudeView.buffer.Len(), "claude must have exactly 10 lines")
	assert.Equal(t, 10, codexView.buffer.Len(), "codex must have exactly 10 lines")

	// Each agent's buffer must contain only its own lines.
	claudeLines := claudeView.buffer.Lines()
	for i, l := range claudeLines {
		assert.Equal(t, fmt.Sprintf("claude line %d", i), l,
			"claude buffer line %d must be from claude", i)
	}

	codexLines := codexView.buffer.Lines()
	for i, l := range codexLines {
		assert.Equal(t, fmt.Sprintf("codex line %d", i), l,
			"codex buffer line %d must be from codex", i)
	}
}

// ---------------------------------------------------------------------------
// Edge case tests
// ---------------------------------------------------------------------------

// TestEdgeCase_LongLine verifies that an output line longer than the panel
// width still appends to the buffer without error.
func TestEdgeCase_LongLine(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	longLine := strings.Repeat("x", 10_000)
	ap = sendOutput(ap, "claude", longLine)

	av, ok := ap.agents["claude"]
	require.True(t, ok)
	lines := av.buffer.Lines()
	require.Len(t, lines, 1)
	assert.Equal(t, longLine, lines[0], "long line must be stored verbatim in the buffer")
}

// TestEdgeCase_ANSIPassthrough verifies that agent output containing ANSI
// escape codes is stored verbatim in the buffer (the TUI layer, not the buffer,
// is responsible for rendering).
func TestEdgeCase_ANSIPassthrough(t *testing.T) {
	t.Parallel()

	ansiLine := "\x1b[32mgreen text\x1b[0m"

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", ansiLine)

	av, ok := ap.agents["claude"]
	require.True(t, ok)
	lines := av.buffer.Lines()
	require.Len(t, lines, 1)
	assert.Equal(t, ansiLine, lines[0], "ANSI escape codes must pass through the buffer unmodified")
}

// TestEdgeCase_TabCharacters verifies that tab characters in agent output are
// stored raw in the buffer and normalised to four spaces in the viewport
// content (via rebuildContent).
func TestEdgeCase_TabCharacters(t *testing.T) {
	t.Parallel()

	tabLine := "col1\tcol2\tcol3"

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", tabLine)

	av, ok := ap.agents["claude"]
	require.True(t, ok)

	// Buffer stores the raw line with tab characters.
	lines := av.buffer.Lines()
	require.Len(t, lines, 1)
	assert.Equal(t, tabLine, lines[0], "buffer must store raw tab characters")

	// Viewport content has tabs replaced with four spaces.
	vpContent := av.viewport.View()
	assert.Contains(t, vpContent, "    ", "viewport content must have tabs replaced with 4 spaces")
	assert.NotContains(t, vpContent, "\t", "viewport content must not contain raw tab characters")
}

// TestEdgeCase_ReactivatedAgent verifies that receiving output for an agent
// that previously reached AgentCompleted status still appends to its buffer
// correctly (no special handling needed; the buffer is always open).
func TestEdgeCase_ReactivatedAgent(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)

	// Mark agent as completed.
	ap = sendStatus(ap, "claude", AgentCompleted, "T-001", "done")

	// Send new output (reactivated).
	ap = sendOutput(ap, "claude", "restarted output")

	av, ok := ap.agents["claude"]
	require.True(t, ok)
	lines := av.buffer.Lines()
	require.NotEmpty(t, lines, "buffer must contain lines after reactivation")
	assert.Equal(t, "restarted output", lines[len(lines)-1])
}

// TestEdgeCase_EmptyLine verifies that an empty string is correctly appended
// to the buffer (empty lines are valid log output).
func TestEdgeCase_EmptyLine(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", "before")
	ap = sendOutput(ap, "claude", "")
	ap = sendOutput(ap, "claude", "after")

	av, ok := ap.agents["claude"]
	require.True(t, ok)
	lines := av.buffer.Lines()
	require.Len(t, lines, 3)
	assert.Equal(t, "before", lines[0])
	assert.Equal(t, "", lines[1], "empty line must be stored in buffer")
	assert.Equal(t, "after", lines[2])
}

// TestEdgeCase_AllAgentsCompleted verifies that when all agents reach
// AgentCompleted status the panel still renders the last active agent's output.
func TestEdgeCase_AllAgentsCompleted(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", "final output")
	ap = sendStatus(ap, "claude", AgentCompleted, "T-001", "done")

	// Panel must still show claude's output.
	assert.Equal(t, "claude", ap.ActiveAgent(), "completed agent must still be the active agent")

	output := stripANSIPanel(ap.View())
	assert.Contains(t, output, "claude", "completed agent name must still appear in view")
}

// TestEdgeCase_TabKeyPassthrough_SingleAgent verifies that when only one agent
// is registered, pressing Tab returns a non-nil Cmd that yields a tea.KeyMsg
// with Type == tea.KeyTab (to pass focus to the parent model).
func TestEdgeCase_TabKeyPassthrough_SingleAgent(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendOutput(ap, "claude", "only agent")

	require.Len(t, ap.agentOrder, 1, "must have exactly one agent for passthrough test")

	_, cmd := pressKey(ap, tea.KeyTab)
	require.NotNil(t, cmd, "Tab with single agent must return a non-nil Cmd")

	// Execute the command and verify it yields a KeyTab message.
	msg := cmd()
	keyMsg, ok := msg.(tea.KeyMsg)
	require.True(t, ok, "cmd must return a tea.KeyMsg")
	assert.Equal(t, tea.KeyTab, keyMsg.Type, "passthrough cmd must carry KeyTab type")
}

// ---------------------------------------------------------------------------
// Additional coverage: FocusChangedMsg
// ---------------------------------------------------------------------------

// TestUpdate_FocusChangedMsg_FocusesPanel verifies that a FocusChangedMsg with
// FocusAgentPanel sets focused=true and one targeting a different panel sets it
// to false.
func TestUpdate_FocusChangedMsg_FocusesPanel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		panel       FocusPanel
		wantFocused bool
	}{
		{"agent panel focused", FocusAgentPanel, true},
		{"sidebar focused", FocusSidebar, false},
		{"event log focused", FocusEventLog, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ap := NewAgentPanelModel(DefaultTheme())
			ap.SetDimensions(80, 20)

			updated, _ := ap.Update(FocusChangedMsg{Panel: tt.panel})
			assert.Equal(t, tt.wantFocused, updated.focused,
				"focused must be %v when FocusChangedMsg.Panel=%v", tt.wantFocused, tt.panel)
		})
	}
}

// ---------------------------------------------------------------------------
// Additional coverage: SetDimensions
// ---------------------------------------------------------------------------

// TestSetDimensions_UpdatesViewportHeight verifies that SetDimensions
// propagates the correct viewport height to all agent viewports.
func TestSetDimensions_UpdatesViewportHeight(t *testing.T) {
	t.Parallel()

	ap := NewAgentPanelModel(DefaultTheme())
	ap.SetFocused(true)
	ap = sendOutput(ap, "claude", "line")

	// SetDimensions after adding an agent.
	ap.SetDimensions(80, 20)

	av, ok := ap.agents["claude"]
	require.True(t, ok)
	// With 1 agent: overhead = 1 (header). vpHeight = 20 - 1 = 19.
	assert.Equal(t, 19, av.viewport.Height, "viewport height must be height - 1 for single agent")
	assert.Equal(t, 80, av.viewport.Width, "viewport width must match panel width")
}

// TestSetDimensions_TwoAgents_ViewportHeight verifies that with 2+ agents the
// tab bar row is also subtracted from the viewport height.
func TestSetDimensions_TwoAgents_ViewportHeight(t *testing.T) {
	t.Parallel()

	ap := NewAgentPanelModel(DefaultTheme())
	ap.SetFocused(true)
	ap = sendOutput(ap, "claude", "line")
	ap = sendOutput(ap, "codex", "line")

	ap.SetDimensions(80, 20)

	// With 2 agents: overhead = 2 (tab bar + header). vpHeight = 20 - 2 = 18.
	claudeView := ap.agents["claude"]
	require.NotNil(t, claudeView)
	assert.Equal(t, 18, claudeView.viewport.Height,
		"viewport height must be height - 2 when tab bar is present")
}

// ---------------------------------------------------------------------------
// Additional coverage: view when dimensions are zero
// ---------------------------------------------------------------------------

// TestView_NoDimensions_ReturnsEmpty verifies that View() returns an empty
// string when width and height have not been set.
func TestView_NoDimensions_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	ap := NewAgentPanelModel(DefaultTheme())
	// Do NOT call SetDimensions.
	output := ap.View()
	assert.Equal(t, "", output, "View must return empty string when dimensions are zero")
}

// ---------------------------------------------------------------------------
// Additional coverage: tab key when unfocused
// ---------------------------------------------------------------------------

// TestUpdate_KeyTab_WhenUnfocused_NoSwitch verifies that keyboard events are
// ignored when the panel does not have focus.
func TestUpdate_KeyTab_WhenUnfocused_NoSwitch(t *testing.T) {
	t.Parallel()

	ap := NewAgentPanelModel(DefaultTheme())
	ap.SetDimensions(80, 20)
	// Explicitly not focused.
	ap.SetFocused(false)

	ap = sendOutput(ap, "claude", "a")
	ap = sendOutput(ap, "codex", "b")

	initialTab := ap.activeTab

	updated, cmd := ap.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, initialTab, updated.activeTab, "Tab when unfocused must not switch tabs")
	assert.Nil(t, cmd, "Tab when unfocused must return nil cmd")
}

// ---------------------------------------------------------------------------
// Additional coverage: agentHeaderView reflects task name
// ---------------------------------------------------------------------------

// TestAgentHeaderView_ShowsTask verifies that the agent header includes the
// task name when one is set via AgentStatusMsg.
func TestAgentHeaderView_ShowsTask(t *testing.T) {
	t.Parallel()

	ap := makePanel(t, 80, 20)
	ap = sendStatus(ap, "claude", AgentRunning, "T-042", "working hard")

	header := stripANSIPanel(ap.agentHeaderView())
	assert.Contains(t, header, "claude", "header must contain agent name")
	assert.Contains(t, header, "T-042", "header must contain task ID")
}
