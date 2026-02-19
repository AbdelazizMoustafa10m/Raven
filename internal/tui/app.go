package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AbdelazizMoustafa10m/Raven/internal/logging"
	"github.com/AbdelazizMoustafa10m/Raven/internal/loop"
	"github.com/AbdelazizMoustafa10m/Raven/internal/workflow"
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

	// Ctx is the cancellation context for backend operations. When nil,
	// a background context is used.
	Ctx context.Context
	// Cancel cancels the Ctx context. Called on graceful shutdown.
	Cancel context.CancelFunc

	// WorkflowEvents is the channel on which the workflow engine broadcasts
	// WorkflowEvent values. May be nil when no workflow is active.
	WorkflowEvents <-chan workflow.WorkflowEvent
	// LoopEvents is the channel on which the implementation loop broadcasts
	// LoopEvent values. May be nil when no loop is active.
	LoopEvents <-chan loop.LoopEvent
	// AgentOutput is the channel on which agent output lines are sent.
	// May be nil when no agents are running.
	AgentOutput <-chan AgentOutputMsg
	// TaskProgress is the channel on which task progress updates are sent.
	// May be nil when no task tracking is active.
	TaskProgress <-chan TaskProgressMsg

	// Engine is the workflow engine reference. May be nil in idle mode.
	Engine *workflow.Engine
}

// PipelineStartMsg is dispatched when the wizard completes to trigger
// pipeline execution with the collected configuration.
type PipelineStartMsg struct {
	// Config is the pipeline configuration collected from the wizard.
	Config PipelineWizardConfig
}

// App is the top-level Bubble Tea model for the Raven Command Center.
// It implements tea.Model (Init, Update, View) and composes all TUI
// sub-models: sidebar, agent panel, event log, status bar, wizard, and
// help overlay.
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

	// Layout manager: computes panel dimensions on resize.
	layout Layout

	// Sub-models
	sidebar    SidebarModel
	agentPanel AgentPanelModel
	eventLog   EventLogModel
	statusBar  StatusBarModel
	wizard     WizardModel
	theme      Theme

	// Backend integration
	bridge         EventBridge
	ctx            context.Context
	cancel         context.CancelFunc
	workflowEvents <-chan workflow.WorkflowEvent
	loopEvents     <-chan loop.LoopEvent
	agentOutput    <-chan AgentOutputMsg
	taskProgress   <-chan TaskProgressMsg
}

// NewApp constructs an App with sensible defaults:
// focus is on the sidebar, ready and quitting are false.
// All sub-models are initialised with the default theme. The sidebar
// receives initial focus to match the default FocusSidebar state.
// If cfg carries event channels, the App wires them through an EventBridge
// so that backend events are forwarded as TUI messages.
func NewApp(cfg AppConfig) App {
	km := DefaultKeyMap()
	theme := DefaultTheme()

	sidebar := NewSidebarModel(theme)
	sidebar.SetFocused(true)

	ctx := cfg.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	return App{
		config:         cfg,
		focus:          FocusSidebar,
		ready:          false,
		quitting:       false,
		keyMap:         km,
		helpOverlay:    NewHelpOverlay(theme, km),
		layout:         NewLayout(),
		sidebar:        sidebar,
		agentPanel:     NewAgentPanelModel(theme),
		eventLog:       NewEventLogModel(theme),
		statusBar:      NewStatusBarModel(theme),
		wizard:         NewWizardModel(theme, nil, nil),
		theme:          theme,
		bridge:         NewEventBridge(),
		ctx:            ctx,
		cancel:         cfg.Cancel,
		workflowEvents: cfg.WorkflowEvents,
		loopEvents:     cfg.LoopEvents,
		agentOutput:    cfg.AgentOutput,
		taskProgress:   cfg.TaskProgress,
	}
}

// Init returns a batch of commands that start draining backend event
// channels via the EventBridge. Each bridge command reads a single event
// from its channel and converts it into a TUI message; the Update handler
// re-invokes the bridge command to keep draining. If no channels are
// configured the returned batch is empty (nil commands are filtered out).
func (a App) Init() tea.Cmd {
	var cmds []tea.Cmd
	if a.workflowEvents != nil {
		cmds = append(cmds, a.bridge.WorkflowEventCmd(a.ctx, a.workflowEvents))
	}
	if a.loopEvents != nil {
		cmds = append(cmds, a.bridge.LoopEventCmd(a.ctx, a.loopEvents))
	}
	if a.agentOutput != nil {
		cmds = append(cmds, a.bridge.AgentOutputCmd(a.ctx, a.agentOutput))
	}
	if a.taskProgress != nil {
		cmds = append(cmds, a.bridge.TaskProgressCmd(a.ctx, a.taskProgress))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// Update dispatches incoming messages and returns the updated model plus any
// follow-up command. It handles window resizing, the help overlay, keyboard
// bindings, and all sub-model message routing.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		return a.handleWindowSize(m)

	case tea.KeyMsg:
		return a.handleKey(m)

	case WizardCompleteMsg:
		// Wizard has completed; log the event and trigger pipeline execution
		// with the collected configuration.
		a.eventLog.AddEntry(EventInfo, "Pipeline wizard completed")
		cfg := m.Config
		return a, func() tea.Msg { return PipelineStartMsg{Config: cfg} }

	case PipelineStartMsg:
		a.eventLog.AddEntry(EventInfo, fmt.Sprintf(
			"Pipeline starting: mode=%s, impl=%s",
			m.Config.PhaseMode, m.Config.ImplAgent,
		))
		return a, nil

	case WizardCancelledMsg:
		a.eventLog.AddEntry(EventInfo, "Pipeline wizard cancelled")
		return a, nil

	case FocusChangedMsg:
		a.focus = m.Panel
		var cmds []tea.Cmd
		var sCmd, apCmd, elCmd tea.Cmd
		a.sidebar, sCmd = a.sidebar.Update(m)
		a.agentPanel, apCmd = a.agentPanel.Update(m)
		a.eventLog, elCmd = a.eventLog.Update(m)
		cmds = append(cmds, sCmd, apCmd, elCmd)
		return a, tea.Batch(cmds...)

	case AgentOutputMsg:
		var cmd tea.Cmd
		a.agentPanel, cmd = a.agentPanel.Update(m)
		// Re-invoke the bridge to keep draining the agent output channel.
		var cmds []tea.Cmd
		cmds = append(cmds, cmd)
		if a.agentOutput != nil {
			cmds = append(cmds, a.bridge.AgentOutputCmd(a.ctx, a.agentOutput))
		}
		return a, tea.Batch(cmds...)

	case AgentStatusMsg:
		var apCmd, elCmd tea.Cmd
		a.agentPanel, apCmd = a.agentPanel.Update(m)
		a.eventLog, elCmd = a.eventLog.Update(m)
		return a, tea.Batch(apCmd, elCmd)

	case WorkflowEventMsg:
		var sCmd, elCmd tea.Cmd
		a.sidebar, sCmd = a.sidebar.Update(m)
		a.eventLog, elCmd = a.eventLog.Update(m)
		a.statusBar = a.statusBar.Update(m)
		// Re-invoke the bridge to keep draining the workflow events channel.
		var cmds []tea.Cmd
		cmds = append(cmds, sCmd, elCmd)
		if a.workflowEvents != nil {
			cmds = append(cmds, a.bridge.WorkflowEventCmd(a.ctx, a.workflowEvents))
		}
		return a, tea.Batch(cmds...)

	case LoopEventMsg:
		var sCmd, elCmd tea.Cmd
		a.sidebar, sCmd = a.sidebar.Update(m)
		a.eventLog, elCmd = a.eventLog.Update(m)
		a.statusBar = a.statusBar.Update(m)
		// Re-invoke the bridge to keep draining the loop events channel.
		var cmds []tea.Cmd
		cmds = append(cmds, sCmd, elCmd)
		if a.loopEvents != nil {
			cmds = append(cmds, a.bridge.LoopEventCmd(a.ctx, a.loopEvents))
		}
		return a, tea.Batch(cmds...)

	case RateLimitMsg:
		var sCmd, elCmd tea.Cmd
		a.sidebar, sCmd = a.sidebar.Update(m)
		a.eventLog, elCmd = a.eventLog.Update(m)
		// RateLimitMsg originates from the loop events bridge (convertLoopEvent).
		// Re-invoke the bridge to keep draining the loop events channel.
		var cmds []tea.Cmd
		cmds = append(cmds, sCmd, elCmd)
		if a.loopEvents != nil {
			cmds = append(cmds, a.bridge.LoopEventCmd(a.ctx, a.loopEvents))
		}
		return a, tea.Batch(cmds...)

	case TaskProgressMsg:
		var sCmd tea.Cmd
		a.sidebar, sCmd = a.sidebar.Update(m)
		// Re-invoke the bridge to keep draining the task progress channel.
		var cmds []tea.Cmd
		cmds = append(cmds, sCmd)
		if a.taskProgress != nil {
			cmds = append(cmds, a.bridge.TaskProgressCmd(a.ctx, a.taskProgress))
		}
		return a, tea.Batch(cmds...)

	case ErrorMsg:
		var cmd tea.Cmd
		a.eventLog, cmd = a.eventLog.Update(m)
		return a, cmd

	case TickMsg:
		var sCmd, elCmd tea.Cmd
		a.sidebar, sCmd = a.sidebar.Update(m)
		a.eventLog, elCmd = a.eventLog.Update(m)
		a.statusBar = a.statusBar.Update(m)
		return a, tea.Batch(sCmd, elCmd)
	}

	return a, nil
}

// handleWindowSize processes tea.WindowSizeMsg, resizes the layout and all
// sub-models, and sets the ready flag.
func (a App) handleWindowSize(m tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	a.width = m.Width
	a.height = m.Height
	a.ready = true

	a.helpOverlay.SetDimensions(m.Width, m.Height)
	a.layout.Resize(m.Width, m.Height)

	// Apply computed dimensions to each sub-model.
	a.sidebar.SetDimensions(a.layout.Sidebar.Width, a.layout.Sidebar.Height)
	a.agentPanel.SetDimensions(a.layout.AgentPanel.Width, a.layout.AgentPanel.Height)
	a.eventLog.SetDimensions(a.layout.EventLog.Width, a.layout.EventLog.Height)
	a.statusBar.SetWidth(m.Width)

	return a, nil
}

// handleKey processes tea.KeyMsg, dispatching to the help overlay, wizard,
// global key bindings, and finally the focused sub-model's key handler.
func (a App) handleKey(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	// When the help overlay is visible, delegate all key events to it.
	if a.helpOverlay.IsVisible() {
		var cmd tea.Cmd
		a.helpOverlay, cmd = a.helpOverlay.Update(m)
		return a, cmd
	}

	// When the wizard is active, forward all keys to it.
	if a.wizard.IsActive() {
		var cmd tea.Cmd
		a.wizard, cmd = a.wizard.Update(m)
		return a, cmd
	}

	switch {
	case key.Matches(m, a.keyMap.Help):
		a.helpOverlay.Toggle()
		return a, nil

	case key.Matches(m, a.keyMap.Quit):
		a.quitting = true
		// Cancel the backend context so bridge goroutines and any running
		// agents/workflows receive a cancellation signal before the TUI exits.
		if a.cancel != nil {
			a.cancel()
		}
		return a, tea.Quit

	case key.Matches(m, a.keyMap.FocusNext):
		a.focus = NextFocus(a.focus)
		a.sidebar.SetFocused(a.focus == FocusSidebar)
		a.agentPanel.SetFocused(a.focus == FocusAgentPanel)
		a.eventLog.SetFocused(a.focus == FocusEventLog)
		return a, func() tea.Msg { return FocusChangedMsg{Panel: a.focus} }

	case key.Matches(m, a.keyMap.FocusPrev):
		a.focus = PrevFocus(a.focus)
		a.sidebar.SetFocused(a.focus == FocusSidebar)
		a.agentPanel.SetFocused(a.focus == FocusAgentPanel)
		a.eventLog.SetFocused(a.focus == FocusEventLog)
		return a, func() tea.Msg { return FocusChangedMsg{Panel: a.focus} }

	case key.Matches(m, a.keyMap.Pause):
		return a, func() tea.Msg { return PauseRequestMsg{} }

	case key.Matches(m, a.keyMap.Skip):
		return a, func() tea.Msg { return SkipRequestMsg{} }

	case key.Matches(m, a.keyMap.ToggleLog):
		var cmd tea.Cmd
		a.eventLog, cmd = a.eventLog.Update(m)
		return a, cmd

	// Forward scrolling / navigation keys to the focused panel.
	case key.Matches(m, a.keyMap.Up),
		key.Matches(m, a.keyMap.Down),
		key.Matches(m, a.keyMap.PageUp),
		key.Matches(m, a.keyMap.PageDown),
		key.Matches(m, a.keyMap.Home),
		key.Matches(m, a.keyMap.End):
		return a.forwardKeyToFocused(m)
	}

	return a, nil
}

// forwardKeyToFocused routes a keyboard event to whichever panel currently
// holds focus. Unmatched focus values are silently ignored.
func (a App) forwardKeyToFocused(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch a.focus {
	case FocusSidebar:
		a.sidebar, cmd = a.sidebar.Update(m)
	case FocusAgentPanel:
		a.agentPanel, cmd = a.agentPanel.Update(m)
	case FocusEventLog:
		a.eventLog, cmd = a.eventLog.Update(m)
	}
	return a, cmd
}

// View renders the complete UI as a string.
//
// Rendering logic:
//   - If quitting, return an empty string to clear the screen on exit.
//   - If not yet ready (no WindowSizeMsg received), show an initializing message.
//   - If the terminal is too small, show a resize warning via the layout.
//   - If the wizard is active, render the wizard overlay.
//   - If the help overlay is visible, render it on top of the full view.
//   - Otherwise, render the full composited layout.
func (a App) View() string {
	if a.quitting {
		return ""
	}

	if !a.ready {
		return "Initializing Raven..."
	}

	if a.width < MinTerminalWidth || a.height < MinTerminalHeight {
		return a.layout.RenderTooSmall(a.theme)
	}

	if a.wizard.IsActive() {
		return a.wizard.View()
	}

	if a.helpOverlay.IsVisible() {
		return a.helpOverlay.View()
	}

	return a.fullView()
}

// fullView renders the complete TUI layout using the layout manager and all
// integrated sub-model views.
func (a App) fullView() string {
	titleBar := a.renderTitleBar()
	sidebar := a.sidebar.View()
	agentPanel := a.agentPanel.View()
	eventLog := a.eventLog.View()
	statusBar := a.statusBar.View()

	return a.layout.Render(a.theme, titleBar, sidebar, agentPanel, eventLog, statusBar)
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
