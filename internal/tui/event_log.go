package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MaxEventLogEntries is the maximum number of entries retained in the event
// log. When the buffer is full the oldest entry is evicted to make room.
const MaxEventLogEntries = 500

// ---------------------------------------------------------------------------
// EventCategory
// ---------------------------------------------------------------------------

// EventCategory classifies an event log entry for colour-coded display.
type EventCategory int

const (
	// EventInfo is the default category for informational messages.
	EventInfo EventCategory = iota
	// EventSuccess indicates a successful operation.
	EventSuccess
	// EventWarning indicates a cautionary condition such as a rate limit.
	EventWarning
	// EventError indicates a failure.
	EventError
	// EventDebug is reserved for low-priority diagnostic messages.
	EventDebug
)

// ---------------------------------------------------------------------------
// EventEntry
// ---------------------------------------------------------------------------

// EventEntry is a single entry in the event log ring buffer.
type EventEntry struct {
	// Timestamp records when the event occurred.
	Timestamp time.Time
	// Category classifies the entry for display purposes.
	Category EventCategory
	// Message is the human-readable description of the event.
	Message string
}

// ---------------------------------------------------------------------------
// EventLogModel
// ---------------------------------------------------------------------------

// EventLogModel is the Bubble Tea sub-model for the scrollable event log
// panel rendered in the lower-right area of the Raven TUI. It maintains a
// bounded ring buffer of EventEntry values and drives a bubbles/viewport for
// display.
//
// EventLogModel follows Bubble Tea's Elm architecture: Update returns a new
// value, and View is a pure function of the model state.
type EventLogModel struct {
	theme      Theme
	width      int
	height     int
	focused    bool
	visible    bool // toggled by the 'l' key; starts true
	entries    []EventEntry
	viewport   viewport.Model
	autoScroll bool
}

// NewEventLogModel creates an EventLogModel that is visible and has
// auto-scroll enabled. The entries buffer starts empty.
func NewEventLogModel(theme Theme) EventLogModel {
	return EventLogModel{
		theme:      theme,
		visible:    true,
		autoScroll: true,
		viewport:   viewport.New(0, 0),
	}
}

// SetDimensions updates the panel width and height and resizes the internal
// viewport. The viewport height is (height - 1) to reserve one row for the
// panel header.
func (el *EventLogModel) SetDimensions(width, height int) {
	el.width = width
	el.height = height

	vpHeight := height - 1
	if vpHeight < 0 {
		vpHeight = 0
	}
	el.viewport.Width = width
	el.viewport.Height = vpHeight

	// Re-render content at the new width.
	el.rebuildContent()
}

// SetFocused sets whether the event log panel currently holds keyboard focus.
func (el *EventLogModel) SetFocused(focused bool) {
	el.focused = focused
}

// SetVisible shows or hides the event log panel.
func (el *EventLogModel) SetVisible(visible bool) {
	el.visible = visible
}

// IsVisible reports whether the panel is currently shown.
func (el EventLogModel) IsVisible() bool {
	return el.visible
}

// AddEntry appends a new EventEntry to the log. When the buffer exceeds
// MaxEventLogEntries the oldest entry is evicted. The viewport content is
// rebuilt after every insertion and, when autoScroll is enabled, the viewport
// is scrolled to the bottom.
func (el *EventLogModel) AddEntry(category EventCategory, message string) {
	entry := EventEntry{
		Timestamp: time.Now(),
		Category:  category,
		Message:   message,
	}

	el.entries = append(el.entries, entry)

	// Evict oldest entries when over the limit.
	if len(el.entries) > MaxEventLogEntries {
		el.entries = el.entries[len(el.entries)-MaxEventLogEntries:]
	}

	el.rebuildContent()
}

// rebuildContent replaces the viewport content with all formatted entries
// joined by newlines, then auto-scrolls if enabled.
func (el *EventLogModel) rebuildContent() {
	if len(el.entries) == 0 {
		el.viewport.SetContent("")
		return
	}

	lines := make([]string, len(el.entries))
	for i, e := range el.entries {
		lines[i] = el.formatEntry(e)
	}
	el.viewport.SetContent(strings.Join(lines, "\n"))

	if el.autoScroll {
		el.viewport.GotoBottom()
	}
}

// formatEntry renders a single EventEntry as "HH:MM:SS message". The
// timestamp is styled with EventTimestamp (muted colour) and the message is
// styled according to its category.
func (el EventLogModel) formatEntry(entry EventEntry) string {
	ts := el.theme.EventTimestamp.Render(entry.Timestamp.Format("15:04:05"))
	msg := el.categoryStyle(entry.Category).Render(entry.Message)
	return ts + " " + msg
}

// categoryStyle returns the lipgloss style appropriate for the given category.
func (el EventLogModel) categoryStyle(cat EventCategory) lipgloss.Style {
	switch cat {
	case EventSuccess:
		return lipgloss.NewStyle().Foreground(ColorSuccess)
	case EventWarning:
		return lipgloss.NewStyle().Foreground(ColorWarning)
	case EventError:
		return lipgloss.NewStyle().Bold(true).Foreground(ColorError)
	case EventDebug:
		return lipgloss.NewStyle().Foreground(ColorMuted)
	default: // EventInfo
		return el.theme.EventMessage
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update processes incoming tea.Msg values and returns the updated model and
// any follow-up command.
//
// Handled messages:
//   - WorkflowEventMsg  — classified and added to the log
//   - LoopEventMsg      — classified and added to the log
//   - AgentStatusMsg    — classified and added to the log
//   - RateLimitMsg      — added as EventWarning
//   - ErrorMsg          — added as EventError
//   - FocusChangedMsg   — updates the focused flag
//   - tea.KeyMsg "l"    — toggles panel visibility
//   - tea.KeyMsg (navigation when focused) — forwarded to the viewport
func (el EventLogModel) Update(msg tea.Msg) (EventLogModel, tea.Cmd) {
	switch msg := msg.(type) {
	case WorkflowEventMsg:
		cat, text := classifyWorkflowEvent(msg)
		el.AddEntry(cat, text)

	case LoopEventMsg:
		cat, text := classifyLoopEvent(msg)
		el.AddEntry(cat, text)

	case AgentStatusMsg:
		cat, text := classifyAgentStatus(msg)
		el.AddEntry(cat, text)

	case RateLimitMsg:
		provider := msg.Provider
		if provider == "" {
			provider = msg.Agent
		}
		text := fmt.Sprintf("Rate limit: %s, waiting %s", provider, formatCountdown(msg.ResetAfter))
		el.AddEntry(EventWarning, text)

	case ErrorMsg:
		text := msg.Detail
		if text == "" {
			text = msg.Source
		}
		el.AddEntry(EventError, text)

	case FocusChangedMsg:
		el.focused = msg.Panel == FocusEventLog

	case tea.KeyMsg:
		// Toggle visibility regardless of focus.
		if msg.Type == tea.KeyRunes && string(msg.Runes) == "l" {
			el.visible = !el.visible
			return el, nil
		}

		// Navigation keys only when focused.
		if el.focused {
			return el.handleKey(msg)
		}
	}

	return el, nil
}

// handleKey routes navigation key events to the viewport and manages the
// autoScroll flag.
func (el EventLogModel) handleKey(msg tea.KeyMsg) (EventLogModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		el.viewport.ScrollUp(1)
		el.autoScroll = false

	case tea.KeyDown:
		el.viewport.ScrollDown(1)
		if el.viewport.AtBottom() {
			el.autoScroll = true
		}

	case tea.KeyPgUp:
		el.viewport.PageUp()
		el.autoScroll = false

	case tea.KeyPgDown:
		el.viewport.PageDown()
		if el.viewport.AtBottom() {
			el.autoScroll = true
		}

	case tea.KeyEnd:
		el.viewport.GotoBottom()
		el.autoScroll = true

	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			el.viewport.ScrollUp(1)
			el.autoScroll = false
		case "j":
			el.viewport.ScrollDown(1)
			if el.viewport.AtBottom() {
				el.autoScroll = true
			}
		case "g":
			el.viewport.GotoTop()
			el.autoScroll = false
		case "G":
			el.viewport.GotoBottom()
			el.autoScroll = true
		}

	default:
	}

	return el, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the event log panel as a string. It returns an empty string
// when the panel is hidden or when dimensions have not been set. The rendered
// output consists of a one-line header followed by the scrollable viewport.
// When the panel has focus a highlighted border colour is used.
func (el EventLogModel) View() string {
	if !el.visible || el.width <= 0 || el.height <= 0 {
		return ""
	}

	var sb strings.Builder

	// Header line.
	header := el.theme.AgentHeader.Render("Event Log")
	sb.WriteString(header)
	sb.WriteString("\n")

	// Body: placeholder when empty, viewport otherwise.
	if len(el.entries) == 0 {
		placeholder := lipgloss.NewStyle().Foreground(ColorMuted).Render("No events yet")
		sb.WriteString(placeholder)
	} else {
		sb.WriteString(el.viewport.View())
	}

	content := sb.String()

	// Apply the container style. When focused use ColorPrimary border.
	containerStyle := el.theme.EventContainer
	if el.focused {
		containerStyle = containerStyle.
			BorderForeground(ColorPrimary)
	}

	return containerStyle.
		Width(el.width).
		Render(content)
}

// ---------------------------------------------------------------------------
// Classify helpers
// ---------------------------------------------------------------------------

// classifyWorkflowEvent maps a WorkflowEventMsg to an EventCategory and a
// human-readable log message.
func classifyWorkflowEvent(msg WorkflowEventMsg) (EventCategory, string) {
	name := msg.WorkflowName
	if name == "" {
		name = msg.WorkflowID
	}

	cat := EventInfo
	evt := strings.ToLower(msg.Event)
	if strings.Contains(evt, "fail") || strings.Contains(evt, "error") {
		cat = EventError
	}

	var text string
	if msg.PrevStep != "" && msg.Step != "" {
		text = fmt.Sprintf("Workflow '%s' step: %s -> %s", name, msg.PrevStep, msg.Step)
	} else if msg.Step != "" {
		text = fmt.Sprintf("Workflow '%s' step: %s", name, msg.Step)
	} else {
		text = fmt.Sprintf("Workflow '%s' event: %s", name, msg.Event)
	}

	return cat, text
}

// classifyLoopEvent maps a LoopEventMsg to an EventCategory and a
// human-readable log message.
func classifyLoopEvent(msg LoopEventMsg) (EventCategory, string) {
	switch msg.Type {
	case LoopIterationStarted:
		return EventInfo, fmt.Sprintf("Iteration %d started", msg.Iteration)

	case LoopIterationCompleted:
		return EventSuccess, fmt.Sprintf("Iteration %d completed", msg.Iteration)

	case LoopTaskSelected:
		return EventInfo, fmt.Sprintf("Task %s selected for iteration %d", msg.TaskID, msg.Iteration)

	case LoopTaskCompleted:
		return EventSuccess, fmt.Sprintf("Task %s completed", msg.TaskID)

	case LoopTaskBlocked:
		return EventWarning, fmt.Sprintf("Task %s blocked", msg.TaskID)

	case LoopWaitingForRateLimit:
		return EventWarning, "Waiting for rate limit..."

	case LoopResumedAfterWait:
		return EventInfo, "Resumed after rate-limit wait"

	case LoopPhaseComplete:
		return EventSuccess, "Loop completed"

	case LoopError:
		text := "Loop error"
		if msg.Detail != "" {
			text = fmt.Sprintf("Loop error: %s", msg.Detail)
		}
		return EventError, text

	default:
		return EventInfo, fmt.Sprintf("Loop event %d", int(msg.Type))
	}
}

// classifyAgentStatus maps an AgentStatusMsg to an EventCategory and a
// human-readable log message.
func classifyAgentStatus(msg AgentStatusMsg) (EventCategory, string) {
	switch msg.Status {
	case AgentRunning:
		text := fmt.Sprintf("Agent %s started", msg.Agent)
		if msg.Task != "" {
			text = fmt.Sprintf("Agent %s started %s", msg.Agent, msg.Task)
		}
		return EventInfo, text

	case AgentCompleted:
		text := fmt.Sprintf("Agent %s completed", msg.Agent)
		if msg.Task != "" {
			text = fmt.Sprintf("Agent %s completed %s", msg.Agent, msg.Task)
		}
		return EventSuccess, text

	case AgentFailed:
		text := fmt.Sprintf("Agent %s failed", msg.Agent)
		if msg.Detail != "" {
			text = fmt.Sprintf("Agent %s failed: %s", msg.Agent, msg.Detail)
		}
		return EventError, text

	case AgentRateLimited:
		return EventWarning, fmt.Sprintf("Agent %s rate limited", msg.Agent)

	case AgentWaiting:
		return EventWarning, fmt.Sprintf("Agent %s waiting", msg.Agent)

	default: // AgentIdle and unknown values
		return EventInfo, fmt.Sprintf("Agent %s idle", msg.Agent)
	}
}
