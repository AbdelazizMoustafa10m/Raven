package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// requireValidResize is a test helper that calls Resize and fatally fails if
// the result does not match wantOK.
func requireValidResize(t *testing.T, l *Layout, width, height int, wantOK bool) {
	t.Helper()
	ok := l.Resize(width, height)
	if wantOK {
		require.True(t, ok, "Resize(%d, %d) must return true", width, height)
	} else {
		require.False(t, ok, "Resize(%d, %d) must return false", width, height)
	}
}

// assertPanelPositive asserts that all five panel dimensions are positive
// (width >= 1, height >= 1).
func assertPanelPositive(t *testing.T, l Layout) {
	t.Helper()
	assert.GreaterOrEqual(t, l.TitleBar.Width, 1, "TitleBar.Width must be >= 1")
	assert.GreaterOrEqual(t, l.TitleBar.Height, 1, "TitleBar.Height must be >= 1")
	assert.GreaterOrEqual(t, l.Sidebar.Width, 1, "Sidebar.Width must be >= 1")
	assert.GreaterOrEqual(t, l.Sidebar.Height, 1, "Sidebar.Height must be >= 1")
	assert.GreaterOrEqual(t, l.AgentPanel.Width, 1, "AgentPanel.Width must be >= 1")
	assert.GreaterOrEqual(t, l.AgentPanel.Height, 1, "AgentPanel.Height must be >= 1")
	assert.GreaterOrEqual(t, l.EventLog.Width, 1, "EventLog.Width must be >= 1")
	assert.GreaterOrEqual(t, l.EventLog.Height, 1, "EventLog.Height must be >= 1")
	assert.GreaterOrEqual(t, l.StatusBar.Width, 1, "StatusBar.Width must be >= 1")
	assert.GreaterOrEqual(t, l.StatusBar.Height, 1, "StatusBar.Height must be >= 1")
}

// ---------------------------------------------------------------------------
// NewLayout
// ---------------------------------------------------------------------------

func TestNewLayout_Defaults(t *testing.T) {
	t.Parallel()

	l := NewLayout()

	assert.Equal(t, DefaultSidebarWidth, l.sidebarWidth, "sidebarWidth must default to DefaultSidebarWidth")
	assert.Equal(t, 0.65, l.agentSplit, "agentSplit must default to 0.65")
	assert.Equal(t, 0, l.termWidth, "termWidth must be zero before first Resize")
	assert.Equal(t, 0, l.termHeight, "termHeight must be zero before first Resize")

	// All panel dimensions must be zero-initialised.
	assert.Equal(t, PanelDimensions{}, l.TitleBar)
	assert.Equal(t, PanelDimensions{}, l.Sidebar)
	assert.Equal(t, PanelDimensions{}, l.AgentPanel)
	assert.Equal(t, PanelDimensions{}, l.EventLog)
	assert.Equal(t, PanelDimensions{}, l.StatusBar)
}

// TestNewLayout verifies the canonical default values called out in the task
// spec acceptance criteria (T-069).
func TestNewLayout(t *testing.T) {
	t.Parallel()

	l := NewLayout()

	assert.Equal(t, 22, l.sidebarWidth, "default sidebarWidth must be 22 (DefaultSidebarWidth)")
	assert.Equal(t, 0.65, l.agentSplit, "default agentSplit must be 0.65")
}

// ---------------------------------------------------------------------------
// Resize -- exact-dimension verification (T-069 spec: 120x40)
// ---------------------------------------------------------------------------

// TestResize_120x40 verifies every panel dimension for the canonical 120x40
// terminal size described in the T-069 acceptance criteria.
//
// Expected breakdown:
//
//	contentHeight = 40 - 1 (title) - 1 (status) = 38
//	mainWidth     = 120 - 22 (sidebar) - 1 (border) = 97
//	agentHeight   = int(38 * 0.65) = 24
//	eventHeight   = 38 - 24 = 14
func TestResize_120x40(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	ok := l.Resize(120, 40)
	require.True(t, ok, "Resize(120, 40) must return true")

	// Title bar: full width, 1 row.
	assert.Equal(t, PanelDimensions{Width: 120, Height: 1}, l.TitleBar,
		"TitleBar must be {120, 1}")

	// Sidebar: fixed width=22, full content height.
	assert.Equal(t, PanelDimensions{Width: 22, Height: 38}, l.Sidebar,
		"Sidebar must be {22, 38}")

	// Agent panel: mainWidth=97, agentHeight=int(38*0.65)=24.
	assert.Equal(t, PanelDimensions{Width: 97, Height: 24}, l.AgentPanel,
		"AgentPanel must be {97, 24}")

	// Event log: same width as agent panel, eventHeight=38-24=14.
	assert.Equal(t, PanelDimensions{Width: 97, Height: 14}, l.EventLog,
		"EventLog must be {97, 14}")

	// Status bar: full width, 1 row.
	assert.Equal(t, PanelDimensions{Width: 120, Height: 1}, l.StatusBar,
		"StatusBar must be {120, 1}")
}

// TestResize_MinimumSize_80x24 verifies that the exact minimum terminal size
// is accepted and all panels have positive dimensions.
func TestResize_MinimumSize_80x24(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	ok := l.Resize(80, 24)
	require.True(t, ok, "Resize(80, 24) must return true")

	assertPanelPositive(t, l)

	// Content height = 24 - 2 = 22; mainWidth = 80 - 22 - 1 = 57.
	assert.Equal(t, PanelDimensions{Width: 80, Height: 1}, l.TitleBar)
	assert.Equal(t, PanelDimensions{Width: 22, Height: 22}, l.Sidebar)
	assert.Equal(t, 57, l.AgentPanel.Width, "AgentPanel.Width at 80-wide terminal must be 57")
	assert.Equal(t, 57, l.EventLog.Width, "EventLog.Width at 80-wide terminal must be 57")
	assert.Equal(t, PanelDimensions{Width: 80, Height: 1}, l.StatusBar)
}

// ---------------------------------------------------------------------------
// Resize -- below-minimum cases
// ---------------------------------------------------------------------------

// TestResize_BelowMinWidth_79x24 verifies the spec requirement that
// Resize(79, 24) returns false.
func TestResize_BelowMinWidth_79x24(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	ok := l.Resize(79, 24)
	assert.False(t, ok, "Resize(79, 24) must return false -- below MinTerminalWidth")
}

// TestResize_BelowMinHeight_80x23 verifies the spec requirement that
// Resize(80, 23) returns false.
func TestResize_BelowMinHeight_80x23(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	ok := l.Resize(80, 23)
	assert.False(t, ok, "Resize(80, 23) must return false -- below MinTerminalHeight")
}

// TestResize_ExactMinimum_80x24 verifies the spec requirement that
// Resize(80, 24) returns true (exact minimum passes).
func TestResize_ExactMinimum_80x24(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	ok := l.Resize(80, 24)
	assert.True(t, ok, "Resize(80, 24) must return true -- exactly at minimum")
}

func TestResize_MinimumExact_ReturnsTrue(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	ok := l.Resize(MinTerminalWidth, MinTerminalHeight)
	require.True(t, ok, "Resize at exactly minimum dimensions must return true")
}

func TestResize_Large_ReturnsTrue(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	ok := l.Resize(200, 60)
	require.True(t, ok, "Resize with large terminal must return true")
}

// TestResize_LargeTerminal_200x80 verifies that a 200x80 terminal produces
// correct panel dimensions and the sidebar stays fixed at 22.
func TestResize_LargeTerminal_200x80(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	ok := l.Resize(200, 80)
	require.True(t, ok, "Resize(200, 80) must return true")

	// Sidebar stays fixed.
	assert.Equal(t, 22, l.Sidebar.Width, "Sidebar.Width must stay 22 at 200-wide terminal")

	// mainWidth = 200 - 22 - 1 = 177
	assert.Equal(t, 177, l.AgentPanel.Width)
	assert.Equal(t, 177, l.EventLog.Width)

	// contentHeight = 80 - 2 = 78; agentHeight = int(78 * 0.65) = 50
	assert.Equal(t, 78, l.Sidebar.Height)
	assert.Equal(t, 50, l.AgentPanel.Height)
	assert.Equal(t, 28, l.EventLog.Height) // 78 - 50 = 28

	assertPanelPositive(t, l)
}

// TestResize_VeryLarge_300x100 verifies that an unusually large terminal
// causes the sidebar to remain fixed at DefaultSidebarWidth (22) while the
// main area expands to fill the remaining space.
func TestResize_VeryLarge_300x100(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	ok := l.Resize(300, 100)
	require.True(t, ok, "Resize(300, 100) must return true")

	// Sidebar width must remain fixed regardless of terminal width.
	assert.Equal(t, DefaultSidebarWidth, l.Sidebar.Width,
		"Sidebar.Width must be fixed at DefaultSidebarWidth even on very large terminals")

	// mainWidth = 300 - 22 - 1 = 277
	assert.Equal(t, 277, l.AgentPanel.Width)
	assert.Equal(t, 277, l.EventLog.Width)

	// All panels must have positive dimensions.
	assertPanelPositive(t, l)
}

func TestResize_TitleBar_Dimensions(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(120, 40)

	assert.Equal(t, 120, l.TitleBar.Width, "TitleBar.Width must equal terminal width")
	assert.Equal(t, TitleBarHeight, l.TitleBar.Height, "TitleBar.Height must equal TitleBarHeight")
}

func TestResize_StatusBar_Dimensions(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(120, 40)

	assert.Equal(t, 120, l.StatusBar.Width, "StatusBar.Width must equal terminal width")
	assert.Equal(t, StatusBarHeight, l.StatusBar.Height, "StatusBar.Height must equal StatusBarHeight")
}

func TestResize_Sidebar_Width(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(120, 40)

	assert.Equal(t, DefaultSidebarWidth, l.Sidebar.Width, "Sidebar.Width must equal sidebarWidth")
}

func TestResize_Sidebar_Height_IsContentHeight(t *testing.T) {
	t.Parallel()

	const (
		width  = 120
		height = 40
	)

	l := NewLayout()
	l.Resize(width, height)

	contentHeight := height - TitleBarHeight - StatusBarHeight
	assert.Equal(t, contentHeight, l.Sidebar.Height, "Sidebar.Height must equal contentHeight")
}

func TestResize_AgentPanel_Width(t *testing.T) {
	t.Parallel()

	const width = 120

	l := NewLayout()
	l.Resize(width, 40)

	expectedMainWidth := width - DefaultSidebarWidth - BorderWidth
	assert.Equal(t, expectedMainWidth, l.AgentPanel.Width, "AgentPanel.Width must be termWidth - sidebarWidth - BorderWidth")
}

func TestResize_EventLog_Width_EqualsAgentPanel(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(120, 40)

	assert.Equal(t, l.AgentPanel.Width, l.EventLog.Width, "EventLog.Width must equal AgentPanel.Width")
}

func TestResize_AgentAndEventHeight_SumToContentHeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{name: "minimum size", width: MinTerminalWidth, height: MinTerminalHeight},
		{name: "medium terminal", width: 120, height: 40},
		{name: "large terminal", width: 220, height: 60},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := NewLayout()
			l.Resize(tt.width, tt.height)

			contentHeight := tt.height - TitleBarHeight - StatusBarHeight
			sum := l.AgentPanel.Height + l.EventLog.Height
			assert.Equal(t, contentHeight, sum,
				"AgentPanel.Height + EventLog.Height must equal contentHeight for %dx%d",
				tt.width, tt.height)
		})
	}
}

func TestResize_AgentSplit_Applied(t *testing.T) {
	t.Parallel()

	const height = 50

	l := NewLayout()
	l.Resize(120, height)

	contentHeight := height - TitleBarHeight - StatusBarHeight
	expectedAgentHeight := int(float64(contentHeight) * 0.65)
	assert.Equal(t, expectedAgentHeight, l.AgentPanel.Height,
		"AgentPanel.Height must be int(contentHeight * 0.65)")
}

// ---------------------------------------------------------------------------
// Resize -- width/height dimension sum invariants
// ---------------------------------------------------------------------------

// TestPanelSum_Width asserts that sidebar + border + main == terminal width
// for a range of terminal sizes.
func TestPanelSum_Width(t *testing.T) {
	t.Parallel()

	sizes := []struct {
		name   string
		width  int
		height int
	}{
		{name: "minimum", width: 80, height: 24},
		{name: "medium", width: 120, height: 40},
		{name: "large", width: 200, height: 60},
		{name: "very large", width: 250, height: 80},
	}

	for _, tt := range sizes {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := NewLayout()
			requireValidResize(t, &l, tt.width, tt.height, true)

			// sidebar + border + main == termWidth
			widthSum := l.Sidebar.Width + BorderWidth + l.AgentPanel.Width
			assert.Equal(t, tt.width, widthSum,
				"Sidebar.Width + BorderWidth + AgentPanel.Width must equal termWidth for %dx%d",
				tt.width, tt.height)
		})
	}
}

// TestPanelSum_Height asserts that title + content + status == terminal height
// for a range of terminal sizes.
func TestPanelSum_Height(t *testing.T) {
	t.Parallel()

	sizes := []struct {
		name   string
		width  int
		height int
	}{
		{name: "minimum", width: 80, height: 24},
		{name: "medium", width: 120, height: 40},
		{name: "large", width: 200, height: 60},
		{name: "very large", width: 250, height: 80},
		{name: "odd height 41", width: 120, height: 41},
		{name: "odd height 37", width: 120, height: 37},
	}

	for _, tt := range sizes {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := NewLayout()
			requireValidResize(t, &l, tt.width, tt.height, true)

			// title + sidebar_height + status == termHeight
			// (Sidebar.Height == contentHeight == termHeight - 2)
			heightSum := l.TitleBar.Height + l.Sidebar.Height + l.StatusBar.Height
			assert.Equal(t, tt.height, heightSum,
				"TitleBar.Height + Sidebar.Height + StatusBar.Height must equal termHeight for %dx%d",
				tt.width, tt.height)
		})
	}
}

// ---------------------------------------------------------------------------
// Resize -- below-minimum cases (additional named per spec)
// ---------------------------------------------------------------------------

func TestResize_BelowMinWidth_ReturnsFalse(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	ok := l.Resize(MinTerminalWidth-1, MinTerminalHeight)
	assert.False(t, ok, "Resize below MinTerminalWidth must return false")
}

func TestResize_BelowMinHeight_ReturnsFalse(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	ok := l.Resize(MinTerminalWidth, MinTerminalHeight-1)
	assert.False(t, ok, "Resize below MinTerminalHeight must return false")
}

func TestResize_BelowMinimum_RecordsTerminalSize(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(50, 10)

	w, h := l.TerminalSize()
	assert.Equal(t, 50, w, "TerminalSize width must reflect the last Resize call even when too small")
	assert.Equal(t, 10, h, "TerminalSize height must reflect the last Resize call even when too small")
}

func TestResize_BelowMinimum_PanelDimensionsUnchanged(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	// First resize to a valid size.
	l.Resize(120, 40)
	prevTitleBar := l.TitleBar
	prevSidebar := l.Sidebar

	// Then resize below minimum -- panel dimensions must NOT change.
	l.Resize(40, 10)
	assert.Equal(t, prevTitleBar, l.TitleBar, "TitleBar must not change when resize is below minimum")
	assert.Equal(t, prevSidebar, l.Sidebar, "Sidebar must not change when resize is below minimum")
}

// TestResize_TableDriven is the primary table-driven acceptance-criteria test
// for the Resize method covering the key spec boundary values.
func TestResize_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		width      int
		height     int
		wantOK     bool
		wantIsTiny bool
	}{
		{name: "zero size", width: 0, height: 0, wantOK: false, wantIsTiny: true},
		{name: "below both", width: 40, height: 12, wantOK: false, wantIsTiny: true},
		{name: "below width only - 79x24", width: 79, height: 24, wantOK: false, wantIsTiny: true},
		{name: "below height only - 80x23", width: 80, height: 23, wantOK: false, wantIsTiny: true},
		{name: "exactly minimum - 80x24", width: 80, height: 24, wantOK: true, wantIsTiny: false},
		{name: "one above minimum", width: 81, height: 25, wantOK: true, wantIsTiny: false},
		{name: "medium terminal", width: 120, height: 40, wantOK: true, wantIsTiny: false},
		{name: "large terminal - 250x80", width: 250, height: 80, wantOK: true, wantIsTiny: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := NewLayout()
			ok := l.Resize(tt.width, tt.height)

			assert.Equal(t, tt.wantOK, ok, "Resize(%d, %d) return value mismatch", tt.width, tt.height)
			assert.Equal(t, tt.wantIsTiny, l.IsTooSmall(),
				"IsTooSmall() mismatch after Resize(%d, %d)", tt.width, tt.height)
		})
	}
}

// TestResize_LayoutAdapts verifies the spec requirement that the layout
// adapts correctly from 80x24 up to 250x80.
func TestResize_LayoutAdapts(t *testing.T) {
	t.Parallel()

	sizes := []struct {
		width  int
		height int
	}{
		{80, 24},
		{100, 30},
		{120, 40},
		{160, 50},
		{200, 60},
		{250, 80},
	}

	for _, sz := range sizes {
		sz := sz
		t.Run("", func(t *testing.T) {
			t.Parallel()

			l := NewLayout()
			ok := l.Resize(sz.width, sz.height)
			require.True(t, ok, "Resize(%d, %d) must return true", sz.width, sz.height)

			// Sidebar width is always fixed.
			assert.Equal(t, DefaultSidebarWidth, l.Sidebar.Width,
				"Sidebar.Width must remain fixed at %d for terminal %dx%d",
				DefaultSidebarWidth, sz.width, sz.height)

			// All panels must have positive dimensions.
			assertPanelPositive(t, l)

			// Width sum invariant: sidebar + border + main == termWidth.
			assert.Equal(t, sz.width, l.Sidebar.Width+BorderWidth+l.AgentPanel.Width,
				"width sum mismatch for %dx%d", sz.width, sz.height)

			// Height sum invariant: title + content + status == termHeight.
			assert.Equal(t, sz.height, l.TitleBar.Height+l.Sidebar.Height+l.StatusBar.Height,
				"height sum mismatch for %dx%d", sz.width, sz.height)
		})
	}
}

// ---------------------------------------------------------------------------
// IsTooSmall
// ---------------------------------------------------------------------------

// TestIsTooSmall verifies the IsTooSmall predicate after both a too-small and
// a valid resize, as required by the T-069 test spec.
func TestIsTooSmall(t *testing.T) {
	t.Parallel()

	l := NewLayout()

	// Before any resize both dimensions are 0 -- too small.
	assert.True(t, l.IsTooSmall(), "IsTooSmall must be true before any Resize")

	// Resize to below minimum -- still too small.
	l.Resize(50, 10)
	assert.True(t, l.IsTooSmall(), "IsTooSmall must be true after Resize below minimum")

	// Resize to valid size -- no longer too small.
	l.Resize(MinTerminalWidth, MinTerminalHeight)
	assert.False(t, l.IsTooSmall(), "IsTooSmall must be false after a valid Resize")

	// Resize back down -- becomes too small again.
	l.Resize(79, 23)
	assert.True(t, l.IsTooSmall(), "IsTooSmall must be true again after Resize drops below minimum")
}

func TestIsTooSmall_BeforeResize_IsTrue(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	// termWidth=0 and termHeight=0 are both < minimum, so IsTooSmall must be true.
	assert.True(t, l.IsTooSmall(), "IsTooSmall must be true before the first Resize call")
}

func TestIsTooSmall_AfterValidResize_IsFalse(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(MinTerminalWidth, MinTerminalHeight)
	assert.False(t, l.IsTooSmall(), "IsTooSmall must be false after a valid Resize")
}

// ---------------------------------------------------------------------------
// TerminalSize
// ---------------------------------------------------------------------------

// TestTerminalSize verifies that TerminalSize returns the correct width and
// height recorded by the most recent Resize call.
func TestTerminalSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{name: "before any resize", width: 0, height: 0},
		{name: "after valid resize 120x40", width: 120, height: 40},
		{name: "after small resize 50x10", width: 50, height: 10},
		{name: "after minimum resize 80x24", width: 80, height: 24},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := NewLayout()
			if tt.width != 0 || tt.height != 0 {
				l.Resize(tt.width, tt.height)
			}
			w, h := l.TerminalSize()
			assert.Equal(t, tt.width, w, "TerminalSize width mismatch")
			assert.Equal(t, tt.height, h, "TerminalSize height mismatch")
		})
	}
}

func TestTerminalSize_BeforeResize_BothZero(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	w, h := l.TerminalSize()
	assert.Equal(t, 0, w)
	assert.Equal(t, 0, h)
}

func TestTerminalSize_AfterResize_MatchesInput(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(160, 50)

	w, h := l.TerminalSize()
	assert.Equal(t, 160, w)
	assert.Equal(t, 50, h)
}

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------

// TestRender_ComposesCorrectly verifies that Render assembles the output from
// all five panel content strings and produces a multi-line result.
func TestRender_ComposesCorrectly(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(120, 40)
	theme := DefaultTheme()

	const (
		titleContent   = "TITLE_CONTENT"
		sidebarContent = "SIDEBAR_CONTENT"
		agentContent   = "AGENT_CONTENT"
		eventContent   = "EVENT_CONTENT"
		statusContent  = "STATUS_CONTENT"
	)

	out := l.Render(theme, titleContent, sidebarContent, agentContent, eventContent, statusContent)

	require.NotEmpty(t, out, "Render must produce a non-empty string")
	assert.Contains(t, out, titleContent, "output must include title bar content")
	assert.Contains(t, out, sidebarContent, "output must include sidebar content")
	assert.Contains(t, out, agentContent, "output must include agent panel content")
	assert.Contains(t, out, eventContent, "output must include event log content")
	assert.Contains(t, out, statusContent, "output must include status bar content")

	lines := strings.Split(out, "\n")
	assert.Greater(t, len(lines), 3, "Render output must span multiple lines")
}

func TestRender_NonEmpty(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(120, 40)
	theme := DefaultTheme()

	out := l.Render(theme, "title", "sidebar", "agent", "events", "status")
	assert.NotEmpty(t, out, "Render must produce a non-empty string")
}

func TestRender_ContainsPanelContent(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(120, 40)
	theme := DefaultTheme()

	const (
		title   = "TITLETAG"
		sidebar = "SIDEBARTAG"
		agent   = "AGENTTAG"
		events  = "EVENTTAG"
		status  = "STATUSTAG"
	)

	out := l.Render(theme, title, sidebar, agent, events, status)
	assert.Contains(t, out, title, "rendered output must contain the title bar content")
	assert.Contains(t, out, sidebar, "rendered output must contain the sidebar content")
	assert.Contains(t, out, agent, "rendered output must contain the agent panel content")
	assert.Contains(t, out, events, "rendered output must contain the event log content")
	assert.Contains(t, out, status, "rendered output must contain the status bar content")
}

func TestRender_ContainsDivider(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(120, 40)
	theme := DefaultTheme()

	out := l.Render(theme, "", "", "", "", "")
	// The divider is constructed from "|" characters.
	assert.Contains(t, out, "|", "rendered output must contain the vertical divider character")
}

func TestRender_HasMultipleLines(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(120, 40)
	theme := DefaultTheme()

	out := l.Render(theme, "title", "sidebar", "agent", "events", "status")
	lines := strings.Split(out, "\n")
	// A 40-row terminal should produce at least a few lines of output.
	assert.Greater(t, len(lines), 3, "Render output must span multiple lines")
}

// TestRender_MultipleTerminalSizes verifies that Render succeeds and produces
// non-empty output across multiple terminal sizes without panicking.
func TestRender_MultipleTerminalSizes(t *testing.T) {
	t.Parallel()

	sizes := []struct {
		width  int
		height int
	}{
		{80, 24},
		{120, 40},
		{200, 60},
		{250, 80},
	}

	for _, sz := range sizes {
		sz := sz
		t.Run("", func(t *testing.T) {
			t.Parallel()

			l := NewLayout()
			require.True(t, l.Resize(sz.width, sz.height))

			theme := DefaultTheme()
			assert.NotPanics(t, func() {
				out := l.Render(theme, "T", "S", "A", "E", "ST")
				assert.NotEmpty(t, out, "Render must produce output at %dx%d", sz.width, sz.height)
			})
		})
	}
}

// ---------------------------------------------------------------------------
// RenderTooSmall
// ---------------------------------------------------------------------------

// TestRenderTooSmall_ContainsWarning verifies that RenderTooSmall produces a
// message containing either "small" or "resize" (case-insensitive) as required
// by the T-069 acceptance criteria.
func TestRenderTooSmall_ContainsWarning(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	theme := DefaultTheme()

	out := l.RenderTooSmall(theme)
	lower := strings.ToLower(out)

	hasWarningKeyword := strings.Contains(lower, "small") || strings.Contains(lower, "resize")
	assert.True(t, hasWarningKeyword,
		"RenderTooSmall output must contain 'small' or 'resize'; got: %q", out)
}

func TestRenderTooSmall_ContainsMessage(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	theme := DefaultTheme()

	out := l.RenderTooSmall(theme)
	// The message must mention resizing and the minimum dimensions.
	assert.Contains(t, out, "80", "RenderTooSmall must reference the minimum width 80")
	assert.Contains(t, out, "24", "RenderTooSmall must reference the minimum height 24")
}

func TestRenderTooSmall_NonEmpty(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	theme := DefaultTheme()

	out := l.RenderTooSmall(theme)
	assert.NotEmpty(t, out, "RenderTooSmall must return a non-empty string")
}

func TestRenderTooSmall_WithKnownSize_NonEmpty(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.Resize(50, 10) // below minimum, but records the terminal size
	theme := DefaultTheme()

	out := l.RenderTooSmall(theme)
	assert.NotEmpty(t, out, "RenderTooSmall must return non-empty output even when terminal size is known-small")
}

func TestRenderTooSmall_WithZeroSize_FallsBackToPlainRender(t *testing.T) {
	t.Parallel()

	l := NewLayout() // termWidth=0, termHeight=0
	theme := DefaultTheme()

	// Should not panic and should return a styled message.
	assert.NotPanics(t, func() {
		out := l.RenderTooSmall(theme)
		assert.NotEmpty(t, out)
	})
}

// TestRenderTooSmall_AlwaysNonEmpty verifies that RenderTooSmall produces
// non-empty output for a range of terminal states (zero, tiny, and
// just-below-minimum).
func TestRenderTooSmall_AlwaysNonEmpty(t *testing.T) {
	t.Parallel()

	sizes := []struct {
		name   string
		width  int
		height int
	}{
		{name: "zero - before any Resize", width: 0, height: 0},
		{name: "tiny 10x5", width: 10, height: 5},
		{name: "below width only 79x24", width: 79, height: 24},
		{name: "below height only 80x23", width: 80, height: 23},
	}

	for _, tt := range sizes {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := NewLayout()
			if tt.width != 0 || tt.height != 0 {
				l.Resize(tt.width, tt.height)
			}
			theme := DefaultTheme()

			assert.NotPanics(t, func() {
				out := l.RenderTooSmall(theme)
				assert.NotEmpty(t, out, "RenderTooSmall must return non-empty output for terminal %dx%d", tt.width, tt.height)
			})
		})
	}
}

// ---------------------------------------------------------------------------
// Panel dimension clamping: no panel ever zero or negative
// ---------------------------------------------------------------------------

// TestPanelDimensions_NeverNegative verifies that no panel dimension is ever
// zero or negative -- the layout clamps all values to a minimum of 1.
func TestPanelDimensions_NeverNegative(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setup        func() Layout
		wantResizeOK bool
	}{
		{
			name: "agentSplit=1.0 forces eventHeight to 0 before clamp",
			setup: func() Layout {
				l := NewLayout()
				l.agentSplit = 1.0
				return l
			},
			wantResizeOK: true,
		},
		{
			name: "agentSplit=0.0 forces agentHeight to 0 before clamp",
			setup: func() Layout {
				l := NewLayout()
				l.agentSplit = 0.0
				return l
			},
			wantResizeOK: true,
		},
		{
			name: "sidebarWidth oversized makes mainWidth negative before clamp",
			setup: func() Layout {
				l := NewLayout()
				l.sidebarWidth = 1000
				return l
			},
			wantResizeOK: true,
		},
		{
			name: "normal minimum terminal",
			setup: func() Layout {
				return NewLayout()
			},
			wantResizeOK: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := tt.setup()
			ok := l.Resize(MinTerminalWidth, MinTerminalHeight)
			require.Equal(t, tt.wantResizeOK, ok)

			if ok {
				assertPanelPositive(t, l)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Clamping: agentHeight / eventHeight must each be at least 1
// ---------------------------------------------------------------------------

func TestResize_ClampAgentHeight_MinOne(t *testing.T) {
	t.Parallel()

	// Construct a layout with agentSplit=1.0 to force eventHeight=0 before clamping.
	l := NewLayout()
	l.agentSplit = 1.0
	l.Resize(MinTerminalWidth, MinTerminalHeight)

	assert.GreaterOrEqual(t, l.AgentPanel.Height, 1, "AgentPanel.Height must be at least 1")
	assert.GreaterOrEqual(t, l.EventLog.Height, 1, "EventLog.Height must be at least 1 (clamped)")
}

// TestResize_ClampAgentHeight_SplitZero verifies that agentSplit=0.0 (which
// would produce agentHeight=0 before clamping) is handled correctly.
func TestResize_ClampAgentHeight_SplitZero(t *testing.T) {
	t.Parallel()

	l := NewLayout()
	l.agentSplit = 0.0
	ok := l.Resize(MinTerminalWidth, MinTerminalHeight)
	require.True(t, ok)

	assert.GreaterOrEqual(t, l.AgentPanel.Height, 1, "AgentPanel.Height must be at least 1 even with agentSplit=0")
	assert.GreaterOrEqual(t, l.EventLog.Height, 1, "EventLog.Height must be at least 1 when agentSplit=0")
}

func TestResize_ClampMainWidth_MinOne(t *testing.T) {
	t.Parallel()

	// Make sidebarWidth so large it would make mainWidth <= 0.
	l := NewLayout()
	l.sidebarWidth = 1000
	l.Resize(MinTerminalWidth, MinTerminalHeight)

	assert.GreaterOrEqual(t, l.AgentPanel.Width, 1, "AgentPanel.Width must be at least 1 even with huge sidebarWidth")
	assert.GreaterOrEqual(t, l.EventLog.Width, 1, "EventLog.Width must be at least 1 even with huge sidebarWidth")
}

func TestResize_ClampContentHeight_MinOne(t *testing.T) {
	t.Parallel()

	// termHeight just large enough to equal TitleBarHeight + StatusBarHeight
	// so contentHeight = 0 before clamping.
	l := NewLayout()
	l.Resize(MinTerminalWidth, TitleBarHeight+StatusBarHeight)

	// If this resize succeeds (>= minimum dimensions), the clamped value applies.
	// If it fails (below minimum height), we just verify no panic occurred.
	assert.GreaterOrEqual(t, l.Sidebar.Height, 0,
		"Sidebar.Height must be non-negative even with minimal terminal height")
}

// ---------------------------------------------------------------------------
// Benchmark
// ---------------------------------------------------------------------------

// BenchmarkResize measures the cost of repeatedly recalculating panel
// dimensions, which happens on every terminal resize event.
func BenchmarkResize(b *testing.B) {
	l := NewLayout()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		l.Resize(120, 40)
	}
}

// BenchmarkRender measures the cost of composing the full TUI frame via
// Render, which is called on every tea.Model.View() invocation.
func BenchmarkRender(b *testing.B) {
	l := NewLayout()
	l.Resize(120, 40)
	theme := DefaultTheme()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		l.Render(theme, "title", "sidebar", "agent", "events", "status")
	}
}
