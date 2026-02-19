package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MaxOutputLines is the maximum number of output lines retained per agent in
// the ring buffer. Once the buffer is full, the oldest lines are overwritten.
const MaxOutputLines = 1000

// ---------------------------------------------------------------------------
// OutputBuffer
// ---------------------------------------------------------------------------

// OutputBuffer is a fixed-capacity ring buffer for agent output lines.
// When the buffer is full the oldest line is overwritten by the newest.
// The zero value is not usable; always construct via NewOutputBuffer.
type OutputBuffer struct {
	lines []string
	start int // logical ring-buffer start index (not pre-reduced)
	count int // number of valid entries currently in the buffer
	cap   int // maximum capacity
}

// NewOutputBuffer creates an OutputBuffer with the given capacity.
// If capacity is <= 0, it defaults to MaxOutputLines.
func NewOutputBuffer(capacity int) OutputBuffer {
	if capacity <= 0 {
		capacity = MaxOutputLines
	}
	return OutputBuffer{
		lines: make([]string, capacity),
		cap:   capacity,
	}
}

// Append adds a line to the buffer. When the buffer is at capacity the oldest
// line is evicted to make room for the new one.
func (b *OutputBuffer) Append(line string) {
	if b.count < b.cap {
		// Buffer still has room: write at the next free slot.
		b.lines[(b.start+b.count)%b.cap] = line
		b.count++
	} else {
		// Buffer is full: overwrite the oldest slot and advance start.
		b.lines[b.start%b.cap] = line
		b.start = (b.start + 1) % b.cap
		// count stays at cap.
	}
}

// Lines returns a slice of all buffered lines in order from oldest to newest.
// The returned slice is a newly allocated copy; mutations do not affect the
// buffer.
func (b OutputBuffer) Lines() []string {
	if b.count == 0 {
		return nil
	}
	out := make([]string, b.count)
	for i := 0; i < b.count; i++ {
		out[i] = b.lines[(b.start+i)%b.cap]
	}
	return out
}

// Len returns the number of lines currently stored in the buffer.
func (b OutputBuffer) Len() int {
	return b.count
}

// ---------------------------------------------------------------------------
// AgentView
// ---------------------------------------------------------------------------

// AgentView holds the display state for a single agent within the agent panel.
// It owns a viewport for scrollable output and an OutputBuffer for the ring
// buffer of output lines.
type AgentView struct {
	name       string
	status     AgentStatus
	task       string
	detail     string
	viewport   viewport.Model
	buffer     OutputBuffer
	autoScroll bool
}

// newAgentView constructs an AgentView with autoScroll enabled and an
// OutputBuffer of MaxOutputLines capacity.
func newAgentView(name string) *AgentView {
	vp := viewport.New(0, 0)
	return &AgentView{
		name:       name,
		status:     AgentIdle,
		buffer:     NewOutputBuffer(MaxOutputLines),
		viewport:   vp,
		autoScroll: true,
	}
}

// rebuildContent replaces the viewport content with the current buffer lines,
// joined by newlines. Tab characters are normalised to four spaces.
func (av *AgentView) rebuildContent() {
	lines := av.buffer.Lines()
	// Normalise tabs to spaces.
	for i, l := range lines {
		lines[i] = strings.ReplaceAll(l, "\t", "    ")
	}
	av.viewport.SetContent(strings.Join(lines, "\n"))
	if av.autoScroll {
		av.viewport.GotoBottom()
	}
}

// ---------------------------------------------------------------------------
// AgentPanelModel
// ---------------------------------------------------------------------------

// AgentPanelModel manages the tabbed agent output display in the right-hand
// upper panel of the Raven TUI. Each agent gets its own scrollable viewport;
// tabs allow switching between agents when multiple are present.
//
// AgentPanelModel follows Bubble Tea's Elm architecture: Update returns a new
// value, and View is a pure function of the model state.
type AgentPanelModel struct {
	theme      Theme
	width      int
	height     int
	focused    bool
	agents     map[string]*AgentView
	agentOrder []string // insertion-ordered agent names
	activeTab  int
}

// NewAgentPanelModel creates an AgentPanelModel with an empty agent map and
// the default auto-scroll behaviour.
func NewAgentPanelModel(theme Theme) AgentPanelModel {
	return AgentPanelModel{
		theme:  theme,
		agents: make(map[string]*AgentView),
	}
}

// SetDimensions updates the panel width and height and resizes all agent
// viewports accordingly. Agents with autoScroll enabled are scrolled to the
// bottom, and the active agent's content is rebuilt.
func (ap *AgentPanelModel) SetDimensions(width, height int) {
	ap.width = width
	ap.height = height

	vpHeight := ap.viewportHeight()

	for _, av := range ap.agents {
		av.viewport.Width = width
		av.viewport.Height = vpHeight
		if av.autoScroll {
			av.viewport.GotoBottom()
		}
	}

	// Rebuild content for the active agent so the viewport reflects the new
	// dimensions immediately.
	if active := ap.activeAgentView(); active != nil {
		active.rebuildContent()
	}
}

// SetFocused sets whether the agent panel currently holds keyboard focus.
// When false, all keyboard events are ignored.
func (ap *AgentPanelModel) SetFocused(focused bool) {
	ap.focused = focused
}

// ActiveAgent returns the name of the currently displayed agent tab, or an
// empty string when no agents are registered.
func (ap AgentPanelModel) ActiveAgent() string {
	if len(ap.agentOrder) == 0 {
		return ""
	}
	if ap.activeTab < 0 || ap.activeTab >= len(ap.agentOrder) {
		return ap.agentOrder[0]
	}
	return ap.agentOrder[ap.activeTab]
}

// activeAgentView returns the AgentView for the currently active tab,
// or nil when no agents exist.
func (ap AgentPanelModel) activeAgentView() *AgentView {
	name := ap.ActiveAgent()
	if name == "" {
		return nil
	}
	return ap.agents[name]
}

// viewportHeight returns the number of rows available for the viewport given
// the current panel dimensions. The header row is always reserved; the tab bar
// row is additionally reserved when there are 2+ agents.
func (ap AgentPanelModel) viewportHeight() int {
	overhead := 1 // header row
	if len(ap.agentOrder) >= 2 {
		overhead++ // tab bar row
	}
	h := ap.height - overhead
	if h < 0 {
		h = 0
	}
	return h
}

// getOrCreateAgent returns the AgentView for the given name, creating one if
// it does not yet exist and registering it in the ordered list.
func (ap *AgentPanelModel) getOrCreateAgent(name string) *AgentView {
	if av, ok := ap.agents[name]; ok {
		return av
	}
	av := newAgentView(name)
	av.viewport.Width = ap.width
	av.viewport.Height = ap.viewportHeight()
	ap.agents[name] = av
	ap.agentOrder = append(ap.agentOrder, name)
	return av
}

// Update processes incoming tea.Msg values and returns the updated model and
// any follow-up command.
//
// Handled messages:
//   - AgentOutputMsg    — appends a line to the named agent's buffer and
//     updates the viewport if that agent is currently active.
//   - AgentStatusMsg    — updates the named agent's status, task, and detail.
//   - FocusChangedMsg   — updates the focused flag.
//   - tea.KeyMsg        — scrolling and tab-switching when focused.
func (ap AgentPanelModel) Update(msg tea.Msg) (AgentPanelModel, tea.Cmd) {
	switch msg := msg.(type) {
	case AgentOutputMsg:
		ap = ap.handleAgentOutput(msg)

	case AgentStatusMsg:
		ap = ap.handleAgentStatus(msg)

	case FocusChangedMsg:
		ap.focused = msg.Panel == FocusAgentPanel

	case tea.KeyMsg:
		if ap.focused {
			return ap.handleKey(msg)
		}
	}

	return ap, nil
}

// handleAgentOutput appends the line to the named agent's ring buffer.
// If the agent is currently the active tab, the viewport content is rebuilt
// and auto-scroll is applied.
func (ap AgentPanelModel) handleAgentOutput(msg AgentOutputMsg) AgentPanelModel {
	av := ap.getOrCreateAgent(msg.Agent)
	av.buffer.Append(msg.Line)

	// Rebuild the viewport only for the currently visible agent to avoid
	// unnecessary string construction for background agents.
	if ap.ActiveAgent() == msg.Agent {
		av.rebuildContent()
	}

	return ap
}

// handleAgentStatus updates the agent's status, task, and detail fields.
func (ap AgentPanelModel) handleAgentStatus(msg AgentStatusMsg) AgentPanelModel {
	av := ap.getOrCreateAgent(msg.Agent)
	av.status = msg.Status
	av.task = msg.Task
	av.detail = msg.Detail
	return ap
}

// handleKey processes keyboard input when the panel is focused. Returns the
// updated model and an optional command.
//
// When only one agent is registered and the user presses Tab, the key message
// is returned as a command so the parent model can advance focus to the next
// panel instead.
func (ap AgentPanelModel) handleKey(msg tea.KeyMsg) (AgentPanelModel, tea.Cmd) {
	// Guard against out-of-bounds activeTab.
	if ap.activeTab >= len(ap.agentOrder) {
		ap.activeTab = 0
	}

	n := len(ap.agentOrder)

	switch msg.Type {
	case tea.KeyTab:
		if n >= 2 {
			ap.activeTab = (ap.activeTab + 1) % n
			ap.switchToActiveTab()
			return ap, nil
		}
		// Single agent: pass Tab through to parent for focus switching.
		return ap, func() tea.Msg { return msg }

	case tea.KeyShiftTab:
		if n >= 2 {
			ap.activeTab = (ap.activeTab - 1 + n) % n
			ap.switchToActiveTab()
			return ap, nil
		}
		return ap, func() tea.Msg { return msg }

	case tea.KeyDown:
		av := ap.activeAgentView()
		if av != nil {
			av.viewport.ScrollDown(1)
			if av.viewport.AtBottom() {
				av.autoScroll = true
			} else {
				av.autoScroll = false
			}
		}
		return ap, nil

	case tea.KeyUp:
		av := ap.activeAgentView()
		if av != nil {
			av.viewport.ScrollUp(1)
			if av.viewport.AtBottom() {
				av.autoScroll = true
			} else {
				av.autoScroll = false
			}
		}
		return ap, nil

	case tea.KeyPgDown:
		av := ap.activeAgentView()
		if av != nil {
			av.viewport.PageDown()
			if av.viewport.AtBottom() {
				av.autoScroll = true
			} else {
				av.autoScroll = false
			}
		}
		return ap, nil

	case tea.KeyPgUp:
		av := ap.activeAgentView()
		if av != nil {
			av.viewport.PageUp()
			if av.viewport.AtBottom() {
				av.autoScroll = true
			} else {
				av.autoScroll = false
			}
		}
		return ap, nil

	case tea.KeyHome:
		av := ap.activeAgentView()
		if av != nil {
			av.viewport.GotoTop()
			av.autoScroll = false
		}
		return ap, nil

	case tea.KeyEnd:
		av := ap.activeAgentView()
		if av != nil {
			av.viewport.GotoBottom()
			av.autoScroll = true
		}
		return ap, nil

	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "j":
			av := ap.activeAgentView()
			if av != nil {
				av.viewport.ScrollDown(1)
				if av.viewport.AtBottom() {
					av.autoScroll = true
				} else {
					av.autoScroll = false
				}
			}
		case "k":
			av := ap.activeAgentView()
			if av != nil {
				av.viewport.ScrollUp(1)
				if av.viewport.AtBottom() {
					av.autoScroll = true
				} else {
					av.autoScroll = false
				}
			}
		case "g":
			av := ap.activeAgentView()
			if av != nil {
				av.viewport.GotoTop()
				av.autoScroll = false
			}
		case "G":
			av := ap.activeAgentView()
			if av != nil {
				av.viewport.GotoBottom()
				av.autoScroll = true
			}
		case "b":
			av := ap.activeAgentView()
			if av != nil {
				av.viewport.PageUp()
				if av.viewport.AtBottom() {
					av.autoScroll = true
				} else {
					av.autoScroll = false
				}
			}
		}
		return ap, nil

	case tea.KeySpace:
		// Space = page down.
		av := ap.activeAgentView()
		if av != nil {
			av.viewport.PageDown()
			if av.viewport.AtBottom() {
				av.autoScroll = true
			} else {
				av.autoScroll = false
			}
		}
		return ap, nil

	default:
	}

	return ap, nil
}

// switchToActiveTab rebuilds the active agent's viewport content so the
// newly selected tab is rendered with up-to-date output. This is called
// after changing ap.activeTab.
func (ap *AgentPanelModel) switchToActiveTab() {
	// Adjust viewport height: adding the first agent does not change the
	// overhead calculation, but switching between 2+ agents might require
	// height adjustment when dimensions changed since last switch.
	vpHeight := ap.viewportHeight()
	if active := ap.activeAgentView(); active != nil {
		active.viewport.Width = ap.width
		active.viewport.Height = vpHeight
		active.rebuildContent()
	}
}

// ---------------------------------------------------------------------------
// View helpers
// ---------------------------------------------------------------------------

// tabBarView renders the tab bar row when two or more agents are present.
// Each tab shows the agent name; the active tab is rendered with
// AgentTabActive, the rest with AgentTab.
func (ap AgentPanelModel) tabBarView() string {
	var sb strings.Builder
	for i, name := range ap.agentOrder {
		if i == ap.activeTab {
			sb.WriteString(ap.theme.AgentTabActive.Render(name))
		} else {
			sb.WriteString(ap.theme.AgentTab.Render(name))
		}
	}
	return sb.String()
}

// agentHeaderView renders the single-line header for the active agent showing
// the status indicator, agent name, and current task (if any).
func (ap AgentPanelModel) agentHeaderView() string {
	av := ap.activeAgentView()
	if av == nil {
		return ap.theme.AgentHeader.Render("No agent")
	}

	indicator := ap.theme.StatusIndicator(av.status)
	label := av.name
	if av.task != "" {
		label = av.name + "  " + av.task
	}

	return indicator + " " + ap.theme.AgentHeader.Render(label)
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the agent panel as a string. It returns an empty string when
// the panel dimensions have not been set. When no agents are registered it
// shows a centred "Waiting for agents..." placeholder. Otherwise it renders
// an optional tab bar (2+ agents), the agent header, and the scrollable
// viewport output.
func (ap AgentPanelModel) View() string {
	if ap.width <= 0 || ap.height <= 0 {
		return ""
	}

	// Guard out-of-bounds activeTab.
	if ap.activeTab >= len(ap.agentOrder) {
		ap.activeTab = 0
	}

	// No agents registered yet: show a placeholder.
	if len(ap.agentOrder) == 0 {
		placeholder := "Waiting for agents..."
		styled := ap.theme.AgentOutput.Render(placeholder)
		return lipgloss.Place(ap.width, ap.height, lipgloss.Center, lipgloss.Center, styled)
	}

	var sb strings.Builder

	// Tab bar (only when 2+ agents).
	if len(ap.agentOrder) >= 2 {
		sb.WriteString(ap.tabBarView())
		sb.WriteString("\n")
	}

	// Agent header line.
	sb.WriteString(ap.agentHeaderView())
	sb.WriteString("\n")

	// Viewport output.
	av := ap.activeAgentView()
	if av != nil {
		sb.WriteString(av.viewport.View())
	}

	return sb.String()
}
