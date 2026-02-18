package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
)

// FocusPanel identifies which panel currently has keyboard focus.
type FocusPanel int

const (
	// FocusSidebar indicates the sidebar panel has focus.
	FocusSidebar FocusPanel = iota
	// FocusAgentPanel indicates the agent output panel has focus.
	FocusAgentPanel
	// FocusEventLog indicates the event log panel has focus.
	FocusEventLog
)

// AppConfig holds configuration for the TUI application.
type AppConfig struct {
	// Version is the Raven semantic version string (e.g. "2.0.0").
	Version string
	// ProjectName is the name of the current project being managed.
	ProjectName string
	// Channels for receiving events from workflow engine / agents
	// (added as concrete types become available from T-067).
}

// App is the top-level Bubble Tea model for the Raven Command Center.
// It implements tea.Model (Init, Update, View) and holds all top-level
// application state. Sub-model fields are commented-out placeholders;
// they will be populated by subsequent TUI tasks.
type App struct {
	config   AppConfig
	width    int
	height   int
	focus    FocusPanel
	ready    bool // true after first WindowSizeMsg
	quitting bool

	// Keyboard navigation
	keyMap      KeyMap
	helpOverlay HelpOverlay

	// Sub-model placeholders (populated by subsequent tasks)
	// sidebar    SidebarModel    // T-070, T-071, T-072
	// agentPanel AgentPanelModel // T-073
	// eventLog   EventLogModel   // T-074
	// statusBar  StatusBarModel  // T-075
}

// NewApp constructs an App with sensible defaults:
// focus is on the sidebar, ready and quitting are false.
func NewApp(cfg AppConfig) App {
	km := DefaultKeyMap()
	theme := DefaultTheme()
	return App{
		config:      cfg,
		focus:       FocusSidebar,
		ready:       false,
		quitting:    false,
		keyMap:      km,
		helpOverlay: NewHelpOverlay(theme, km),
	}
}

// Init returns nil; bubbletea v1.x automatically sends a WindowSizeMsg on
// startup, so no explicit command is required here.
func (a App) Init() tea.Cmd {
	return nil
}

// Update dispatches incoming messages and returns the updated model plus any
// follow-up command. It handles window resizing, the help overlay, and all
// keyboard bindings. Sub-model messages are forwarded as they become available.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		a.ready = true
		a.helpOverlay.SetDimensions(m.Width, m.Height)
		return a, nil

	case tea.KeyMsg:
		// When the help overlay is visible, delegate all key events to it so
		// that only '?' and 'Esc' are handled (overlay keys suppress others).
		if a.helpOverlay.IsVisible() {
			var cmd tea.Cmd
			a.helpOverlay, cmd = a.helpOverlay.Update(m)
			return a, cmd
		}

		switch {
		case key.Matches(m, a.keyMap.Help):
			a.helpOverlay.Toggle()
			return a, nil

		case key.Matches(m, a.keyMap.Quit):
			a.quitting = true
			return a, tea.Quit

		case key.Matches(m, a.keyMap.FocusNext):
			a.focus = NextFocus(a.focus)
			return a, func() tea.Msg { return FocusChangedMsg{Panel: a.focus} }

		case key.Matches(m, a.keyMap.FocusPrev):
			a.focus = PrevFocus(a.focus)
			return a, func() tea.Msg { return FocusChangedMsg{Panel: a.focus} }

		case key.Matches(m, a.keyMap.Pause):
			return a, func() tea.Msg { return PauseRequestMsg{} }

		case key.Matches(m, a.keyMap.Skip):
			return a, func() tea.Msg { return SkipRequestMsg{} }

		case key.Matches(m, a.keyMap.ToggleLog):
			// ToggleLog is a no-op until the event log sub-model is wired in
			// (T-074/T-078). The key is handled here to prevent it from falling
			// through to the default no-op branch.
			return a, nil

		// Scrolling keys are forwarded to the focused panel's sub-model once
		// sub-models are wired in. For now they are consumed without action.
		case key.Matches(m, a.keyMap.Up),
			key.Matches(m, a.keyMap.Down),
			key.Matches(m, a.keyMap.PageUp),
			key.Matches(m, a.keyMap.PageDown),
			key.Matches(m, a.keyMap.Home),
			key.Matches(m, a.keyMap.End):
			return a, nil
		}
	}

	return a, nil
}

// View renders the complete UI as a string.
//
// Rendering logic:
//   - If quitting, return an empty string to clear the screen on exit.
//   - If not yet ready (no WindowSizeMsg received), show an initializing message.
//   - If the terminal is too small (< 80 wide or < 24 tall), show a resize warning.
//   - If the help overlay is visible, render it on top of the full view.
//   - Otherwise, render the title bar followed by stub panel areas.
func (a App) View() string {
	if a.quitting {
		return ""
	}

	if !a.ready {
		return "Initializing Raven..."
	}

	if a.width < 80 || a.height < 24 {
		return terminalTooSmallView()
	}

	if a.helpOverlay.IsVisible() {
		return a.helpOverlay.View()
	}

	return a.fullView()
}

// terminalTooSmallView returns a centered warning when the terminal is below
// the minimum supported dimensions (80x24).
func terminalTooSmallView() string {
	msg := "Terminal too small. Please resize to at least 80x24."
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("3")). // yellow
		Render(msg)
}

// fullView renders the complete TUI layout: title bar + placeholder panels.
// Real panel sub-models will replace the stubs as T-067 through T-075 land.
func (a App) fullView() string {
	titleBar := a.renderTitleBar()

	// Reserve one row for the title bar; the rest is the panel area.
	panelHeight := a.height - 1
	if panelHeight < 1 {
		panelHeight = 1
	}

	panel := lipgloss.NewStyle().
		Width(a.width).
		Height(panelHeight).
		Render("[panels loading…]")

	return lipgloss.JoinVertical(lipgloss.Left, titleBar, panel)
}

// renderTitleBar builds a full-width title bar showing the Raven version and
// the project name (when available).
func (a App) renderTitleBar() string {
	title := fmt.Sprintf("Raven v%s — Command Center", a.config.Version)
	if a.config.ProjectName != "" {
		title = fmt.Sprintf("%s  |  %s", title, a.config.ProjectName)
	}

	return lipgloss.NewStyle().
		Width(a.width).
		Bold(true).
		Background(lipgloss.Color("62")). // purple
		Foreground(lipgloss.Color("15")). // white
		Padding(0, 1).
		Render(title)
}

// RunTUI creates a tea.Program configured for full-screen rendering with
// cell-motion mouse support, runs it, and returns any error encountered.
//
// Use tea.WithMouseCellMotion (not WithMouseAllMotion) so that the user can
// still select and copy text from the terminal.
func RunTUI(cfg AppConfig) error {
	logger := logging.New("tui")
	logger.Info("starting TUI", "version", cfg.Version, "project", cfg.ProjectName)

	p := tea.NewProgram(
		NewApp(cfg),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}

	return nil
}
