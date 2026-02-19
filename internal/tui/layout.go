package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

// MinTerminalWidth is the minimum terminal width (in columns) required for the
// full TUI layout to render correctly. Below this threshold RenderTooSmall is
// used instead.
const MinTerminalWidth = 80

// MinTerminalHeight is the minimum terminal height (in rows) required for the
// full TUI layout. Below this threshold RenderTooSmall is used instead.
const MinTerminalHeight = 24

// DefaultSidebarWidth is the default fixed column width of the sidebar panel.
const DefaultSidebarWidth = 22

// TitleBarHeight is the number of terminal rows consumed by the title bar.
const TitleBarHeight = 1

// StatusBarHeight is the number of terminal rows consumed by the status bar.
const StatusBarHeight = 1

// BorderWidth is the width (in columns) of the vertical divider between the
// sidebar and the main content area.
const BorderWidth = 1

// ---------------------------------------------------------------------------
// PanelDimensions
// ---------------------------------------------------------------------------

// PanelDimensions holds the computed width and height for a single TUI panel.
// Both values are in terminal cell units (columns / rows). Zero values mean
// the layout has not yet been computed via Resize.
type PanelDimensions struct {
	// Width is the panel width in terminal columns.
	Width int
	// Height is the panel height in terminal rows.
	Height int
}

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

// Layout computes and holds the dimensions of every panel in the Raven TUI.
// It must be updated on every tea.WindowSizeMsg by calling Resize. The
// resulting PanelDimensions fields can then be applied inside View to size
// the lipgloss containers correctly.
//
// Layout diagram:
//
//	+---------------------------------------------------+
//	| Title Bar (1 line)                                 |
//	+---------------+-----------------------------------+
//	| Sidebar       | Agent Panel (upper right)          |
//	| (fixed width) |                                    |
//	|               |-----------------------------------|
//	|               | Event Log (lower right)            |
//	+---------------+-----------------------------------+
//	| Status Bar (1 line)                                |
//	+---------------------------------------------------+
type Layout struct {
	termWidth    int
	termHeight   int
	sidebarWidth int
	agentSplit   float64 // fraction of contentHeight allocated to the agent panel

	// TitleBar holds the computed dimensions for the title bar.
	TitleBar PanelDimensions
	// Sidebar holds the computed dimensions for the sidebar panel.
	Sidebar PanelDimensions
	// AgentPanel holds the computed dimensions for the upper-right agent panel.
	AgentPanel PanelDimensions
	// EventLog holds the computed dimensions for the lower-right event log panel.
	EventLog PanelDimensions
	// StatusBar holds the computed dimensions for the status bar.
	StatusBar PanelDimensions
}

// NewLayout returns a Layout with DefaultSidebarWidth and an agentSplit of
// 0.65 (the agent panel takes 65 % of the content height). All
// PanelDimensions fields are zero-initialised until the first Resize call.
func NewLayout() Layout {
	return Layout{
		sidebarWidth: DefaultSidebarWidth,
		agentSplit:   0.65,
	}
}

// Resize recalculates all PanelDimensions for the given terminal size.
//
// If the terminal is smaller than the minimum supported dimensions
// (MinTerminalWidth x MinTerminalHeight) the method records the raw dimensions
// (so IsTooSmall and TerminalSize can report the actual values) and returns
// false without updating the panel dimensions.
//
// Returns true when the layout was successfully recalculated.
func (l *Layout) Resize(width, height int) bool {
	l.termWidth = width
	l.termHeight = height

	if width < MinTerminalWidth || height < MinTerminalHeight {
		return false
	}

	// Rows available between the title bar and the status bar.
	contentHeight := l.termHeight - TitleBarHeight - StatusBarHeight
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Columns available to the right of the sidebar + divider.
	mainWidth := l.termWidth - l.sidebarWidth - BorderWidth
	if mainWidth < 1 {
		mainWidth = 1
	}

	// Split the content height between the agent panel (top) and event log (bottom).
	agentHeight := int(float64(contentHeight) * l.agentSplit)
	if agentHeight < 1 {
		agentHeight = 1
	}

	eventHeight := contentHeight - agentHeight
	if eventHeight < 1 {
		eventHeight = 1
	}

	l.TitleBar = PanelDimensions{Width: l.termWidth, Height: TitleBarHeight}
	l.Sidebar = PanelDimensions{Width: l.sidebarWidth, Height: contentHeight}
	l.AgentPanel = PanelDimensions{Width: mainWidth, Height: agentHeight}
	l.EventLog = PanelDimensions{Width: mainWidth, Height: eventHeight}
	l.StatusBar = PanelDimensions{Width: l.termWidth, Height: StatusBarHeight}

	return true
}

// IsTooSmall returns true when the last known terminal dimensions fall below
// the minimum supported size (MinTerminalWidth x MinTerminalHeight).
func (l Layout) IsTooSmall() bool {
	return l.termWidth < MinTerminalWidth || l.termHeight < MinTerminalHeight
}

// TerminalSize returns the most recently recorded terminal dimensions in
// (width, height) order. Both values are zero until the first Resize call.
func (l Layout) TerminalSize() (int, int) {
	return l.termWidth, l.termHeight
}

// Render assembles the complete TUI frame from the five pre-rendered content
// strings, applying exact panel sizing and the vertical divider. The theme
// parameter supplies the divider color.
//
// The content strings should be produced by the individual panel sub-models
// (sidebar, agentPanel, etc.) and must NOT already have width/height applied;
// Render sizes them to match the computed PanelDimensions.
func (l Layout) Render(theme Theme, titleBar, sidebar, agentPanel, eventLog, statusBar string) string {
	// Apply exact sizing to each panel content string.
	titleBarView := lipgloss.NewStyle().
		Width(l.TitleBar.Width).
		Height(l.TitleBar.Height).
		Render(titleBar)

	sidebarView := lipgloss.NewStyle().
		Width(l.Sidebar.Width).
		Height(l.Sidebar.Height).
		Render(sidebar)

	agentView := lipgloss.NewStyle().
		Width(l.AgentPanel.Width).
		Height(l.AgentPanel.Height).
		Render(agentPanel)

	eventView := lipgloss.NewStyle().
		Width(l.EventLog.Width).
		Height(l.EventLog.Height).
		Render(eventLog)

	statusView := lipgloss.NewStyle().
		Width(l.StatusBar.Width).
		Height(l.StatusBar.Height).
		Render(statusBar)

	// Build the vertical divider: one "|" per row, spanning the full content height.
	dividerContent := strings.Repeat("|\n", l.Sidebar.Height-1) + "|"
	divider := lipgloss.NewStyle().
		Width(BorderWidth).
		Height(l.Sidebar.Height).
		Foreground(ColorBorder).
		Render(dividerContent)

	// Stack the two right-side panels vertically, then join with sidebar + divider.
	mainArea := lipgloss.JoinVertical(lipgloss.Left, agentView, eventView)
	middle := lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, divider, mainArea)

	return lipgloss.JoinVertical(lipgloss.Left, titleBarView, middle, statusView)
}

// RenderTooSmall returns a message instructing the user to enlarge their
// terminal. When a terminal size has been recorded the message is centered
// within the available area using lipgloss.Place; otherwise the raw
// theme.ErrorText style is applied without placement.
func (l Layout) RenderTooSmall(theme Theme) string {
	msg := "Terminal too small.\nPlease resize to at least 80×24."
	styled := theme.ErrorText.Render(msg)

	if l.termWidth <= 0 || l.termHeight <= 0 {
		return styled
	}

	return lipgloss.Place(l.termWidth, l.termHeight, lipgloss.Center, lipgloss.Center, styled)
}
