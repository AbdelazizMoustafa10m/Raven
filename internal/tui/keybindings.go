package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// KeyMap
// ---------------------------------------------------------------------------

// KeyMap defines all keybindings for the TUI. Fields are grouped by context:
// global keys are always active, scrolling keys are only forwarded to the
// focused viewport panel, and agent-panel keys are only active when the agent
// panel has focus.
type KeyMap struct {
	// Global keys (always active)
	Quit      key.Binding
	Help      key.Binding
	Pause     key.Binding
	Skip      key.Binding
	ToggleLog key.Binding
	FocusNext key.Binding
	FocusPrev key.Binding

	// Scrolling keys (active in focused viewport panel)
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding

	// Agent panel keys (active when agent panel focused)
	NextAgent key.Binding
	PrevAgent key.Binding
}

// DefaultKeyMap returns the default keybinding configuration for the Raven TUI.
// Key names follow the Bubble Tea format ("ctrl+c", "shift+tab", etc.).
func DefaultKeyMap() KeyMap {
	return KeyMap{
		// --- Global ---
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c", "ctrl+q"),
			key.WithHelp("q/ctrl+c", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Pause: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "pause/resume"),
		),
		Skip: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "skip task"),
		),
		ToggleLog: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "toggle log"),
		),
		FocusNext: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next panel"),
		),
		FocusPrev: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev panel"),
		),

		// --- Scrolling ---
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "scroll up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "scroll down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		Home: key.NewBinding(
			key.WithKeys("home"),
			key.WithHelp("home", "go to top"),
		),
		End: key.NewBinding(
			key.WithKeys("end"),
			key.WithHelp("end", "go to bottom"),
		),

		// --- Agent panel ---
		NextAgent: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next agent"),
		),
		PrevAgent: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev agent"),
		),
	}
}

// ---------------------------------------------------------------------------
// Focus cycling
// ---------------------------------------------------------------------------

// focusPanelCount is the total number of focusable panels in the cycle.
const focusPanelCount = 3

// NextFocus returns the next panel in the focus cycle:
// FocusSidebar -> FocusAgentPanel -> FocusEventLog -> FocusSidebar.
func NextFocus(current FocusPanel) FocusPanel {
	return FocusPanel((int(current) + 1) % focusPanelCount)
}

// PrevFocus returns the previous panel in the focus cycle:
// FocusSidebar -> FocusEventLog -> FocusAgentPanel -> FocusSidebar.
func PrevFocus(current FocusPanel) FocusPanel {
	return FocusPanel((int(current) + focusPanelCount - 1) % focusPanelCount)
}

// ---------------------------------------------------------------------------
// Control messages
// ---------------------------------------------------------------------------

// PauseRequestMsg is sent when the user presses 'p' to pause or resume the
// workflow. The workflow integration layer (T-078) handles this message.
type PauseRequestMsg struct{}

// SkipRequestMsg is sent when the user presses 's' to skip the current task.
// The workflow integration layer (T-078) handles this message.
type SkipRequestMsg struct{}

// ---------------------------------------------------------------------------
// HelpOverlay
// ---------------------------------------------------------------------------

// HelpOverlay displays a centered keybinding reference over the TUI.
// It is rendered on top of the existing layout when visible.
type HelpOverlay struct {
	theme   Theme
	keyMap  KeyMap
	visible bool
	width   int
	height  int
}

// NewHelpOverlay creates a HelpOverlay with the given theme and keymap.
// The overlay starts hidden; call Toggle() or SetDimensions to configure it.
func NewHelpOverlay(theme Theme, keyMap KeyMap) HelpOverlay {
	return HelpOverlay{
		theme:  theme,
		keyMap: keyMap,
	}
}

// SetDimensions updates the terminal dimensions used to center the overlay.
func (h *HelpOverlay) SetDimensions(width, height int) {
	h.width = width
	h.height = height
}

// Toggle flips the visibility of the help overlay.
func (h *HelpOverlay) Toggle() {
	h.visible = !h.visible
}

// IsVisible reports whether the overlay is currently shown.
func (h HelpOverlay) IsVisible() bool {
	return h.visible
}

// Update processes key events when the overlay is visible. Pressing '?' or
// 'Esc' dismisses the overlay; all other keys are consumed without action.
func (h HelpOverlay) Update(msg tea.Msg) (HelpOverlay, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(keyMsg, h.keyMap.Help):
			h.visible = false
		case keyMsg.Type == tea.KeyEsc:
			h.visible = false
		}
	}
	return h, nil
}

// View renders the help overlay as a full-screen string. The keybindings are
// grouped into three categories (Navigation, Actions, Scrolling) and presented
// in a bordered, centered box. Returns an empty string when not visible or
// when dimensions are not yet known.
func (h HelpOverlay) View() string {
	if !h.visible || h.width == 0 || h.height == 0 {
		return ""
	}

	content := h.buildContent()

	// Build a bordered box around the content.
	boxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7B78FF"}).
		Padding(1, 2)

	boxed := boxStyle.Render(content)

	// Center the boxed content on the full terminal.
	return lipgloss.Place(
		h.width, h.height,
		lipgloss.Center, lipgloss.Center,
		boxed,
	)
}

// buildContent assembles the keybinding table inside the help overlay box.
func (h HelpOverlay) buildContent() string {
	var sb strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7B78FF"}).
		MarginBottom(1)
	sb.WriteString(titleStyle.Render("Raven — Keyboard Shortcuts"))
	sb.WriteString("\n\n")

	// Section header style.
	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "#10B981", Dark: "#34D399"}).
		MarginTop(1)

	// --- Navigation ---
	sb.WriteString(sectionStyle.Render("Navigation"))
	sb.WriteString("\n")
	sb.WriteString(h.bindingLine(h.keyMap.FocusNext))
	sb.WriteString(h.bindingLine(h.keyMap.FocusPrev))
	sb.WriteString(h.bindingLine(h.keyMap.NextAgent))
	sb.WriteString("\n")

	// --- Actions ---
	sb.WriteString(sectionStyle.Render("Actions"))
	sb.WriteString("\n")
	sb.WriteString(h.bindingLine(h.keyMap.Pause))
	sb.WriteString(h.bindingLine(h.keyMap.Skip))
	sb.WriteString(h.bindingLine(h.keyMap.ToggleLog))
	sb.WriteString(h.bindingLine(h.keyMap.Help))
	sb.WriteString(h.bindingLine(h.keyMap.Quit))
	sb.WriteString("\n")

	// --- Scrolling ---
	sb.WriteString(sectionStyle.Render("Scrolling"))
	sb.WriteString("\n")
	sb.WriteString(h.bindingLine(h.keyMap.Up))
	sb.WriteString(h.bindingLine(h.keyMap.Down))
	sb.WriteString(h.bindingLine(h.keyMap.PageUp))
	sb.WriteString(h.bindingLine(h.keyMap.PageDown))
	sb.WriteString(h.bindingLine(h.keyMap.Home))
	sb.WriteString(h.bindingLine(h.keyMap.End))
	sb.WriteString("\n")

	// Dismiss hint
	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}).
		Italic(true)
	sb.WriteString(hintStyle.Render("Press ? or Esc to close"))

	return sb.String()
}

// bindingLine formats a single key.Binding as "  KEY  description\n".
func (h HelpOverlay) bindingLine(b key.Binding) string {
	k := h.theme.HelpKey.Render(b.Help().Key)
	d := h.theme.HelpDesc.Render(b.Help().Desc)
	return "  " + k + "  " + d + "\n"
}
