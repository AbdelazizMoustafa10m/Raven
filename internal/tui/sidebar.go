package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// WorkflowStatus
// ---------------------------------------------------------------------------

// WorkflowStatus represents the lifecycle state of a workflow for display
// purposes in the sidebar.
type WorkflowStatus int

const (
	// WorkflowIdle means the workflow is known but not currently active.
	WorkflowIdle WorkflowStatus = iota
	// WorkflowRunning means the workflow is actively executing steps.
	WorkflowRunning
	// WorkflowPaused means the workflow has been suspended mid-execution.
	WorkflowPaused
	// WorkflowCompleted means the workflow finished all steps successfully.
	WorkflowCompleted
	// WorkflowFailed means the workflow encountered a terminal error.
	WorkflowFailed
)

// workflowStatusStrings maps each WorkflowStatus constant to its string label.
var workflowStatusStrings = []string{
	"idle",
	"running",
	"paused",
	"completed",
	"failed",
}

// String returns a human-readable label for the WorkflowStatus.
// Returns "unknown" for values outside the defined range.
func (s WorkflowStatus) String() string {
	if int(s) < 0 || int(s) >= len(workflowStatusStrings) {
		return "unknown"
	}
	return workflowStatusStrings[s]
}

// workflowStatusFromEvent maps a WorkflowEventMsg.Event string to a
// WorkflowStatus. Unrecognised event strings map to WorkflowRunning so that
// any observed step transition keeps the workflow visible as active.
func workflowStatusFromEvent(event string) WorkflowStatus {
	switch strings.ToLower(event) {
	case "idle", "stopped", "not_started":
		return WorkflowIdle
	case "paused", "waiting", "rate_limited":
		return WorkflowPaused
	case "completed", "done", "success":
		return WorkflowCompleted
	case "failed", "error":
		return WorkflowFailed
	default:
		// Any step transition that is not one of the terminal states is
		// treated as running.
		return WorkflowRunning
	}
}

// ---------------------------------------------------------------------------
// WorkflowEntry
// ---------------------------------------------------------------------------

// WorkflowEntry holds the display data for a single workflow entry rendered
// in the sidebar workflow list.
type WorkflowEntry struct {
	// ID is the unique identifier used for deduplication.
	ID string
	// Name is the human-readable workflow name.
	Name string
	// Status is the current lifecycle state.
	Status WorkflowStatus
	// StartedAt records when the workflow was first observed.
	StartedAt time.Time
	// Detail is optional context such as the current step or task ID.
	Detail string
}

// ---------------------------------------------------------------------------
// SidebarModel
// ---------------------------------------------------------------------------

// SidebarModel is the Bubble Tea sub-model for the sidebar panel.
// It maintains the workflow list section and provides hooks for the task
// progress (T-071) and rate-limit status (T-072) sections that will be
// added by subsequent tasks.
//
// Update returns (SidebarModel, tea.Cmd) — not (tea.Model, tea.Cmd) — so the
// parent App must store the returned value in its own sidebar field.
type SidebarModel struct {
	theme  Theme
	width  int
	height int

	// focused indicates whether the sidebar currently holds keyboard focus.
	focused bool

	// workflows is the ordered list of tracked workflows.
	workflows []WorkflowEntry
	// workflowIndex maps WorkflowEntry.ID → slice index for O(1) dedup.
	workflowIndex map[string]int
	// selectedIdx is the index of the currently highlighted workflow.
	selectedIdx int
	// scrollOffset is the first visible row index inside the workflow list.
	scrollOffset int

	// Sub-section placeholders populated by T-071 and T-072.
	// taskProgress TaskProgressModel
	// rateLimits   RateLimitModel
}

// NewSidebarModel creates a SidebarModel with the given theme and an empty
// workflow list. Dimensions default to zero until SetDimensions is called.
func NewSidebarModel(theme Theme) SidebarModel {
	return SidebarModel{
		theme:         theme,
		workflowIndex: make(map[string]int),
	}
}

// SetDimensions updates the sidebar panel size. This should be called
// whenever the parent App processes a tea.WindowSizeMsg.
func (m *SidebarModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
}

// SetFocused sets whether the sidebar has keyboard focus. When focused is
// false, navigation key events are ignored.
func (m *SidebarModel) SetFocused(focused bool) {
	m.focused = focused
}

// SelectedWorkflow returns the Name of the currently selected workflow, or an
// empty string when the workflow list is empty.
func (m SidebarModel) SelectedWorkflow() string {
	if len(m.workflows) == 0 {
		return ""
	}
	if m.selectedIdx < 0 || m.selectedIdx >= len(m.workflows) {
		return ""
	}
	return m.workflows[m.selectedIdx].Name
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update processes incoming tea.Msg values and returns the updated model and
// any follow-up command.
//
// Handled messages:
//   - WorkflowEventMsg  — adds or updates a workflow in the list
//   - FocusChangedMsg   — updates the focused flag
//   - tea.KeyMsg        — j/k/up/down navigation when focused
func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	switch msg := msg.(type) {
	case WorkflowEventMsg:
		m = m.handleWorkflowEvent(msg)

	case FocusChangedMsg:
		m.focused = msg.Panel == FocusSidebar

	case tea.KeyMsg:
		if m.focused {
			m = m.handleKeyMsg(msg)
		}
	}

	return m, nil
}

// handleWorkflowEvent adds a new WorkflowEntry or updates the status of an
// existing one. WorkflowID is used as the deduplication key.
func (m SidebarModel) handleWorkflowEvent(msg WorkflowEventMsg) SidebarModel {
	id := msg.WorkflowID
	if id == "" {
		id = msg.WorkflowName
	}

	status := workflowStatusFromEvent(msg.Event)

	if idx, exists := m.workflowIndex[id]; exists {
		// Update in place — create a new slice copy to stay immutable.
		updated := make([]WorkflowEntry, len(m.workflows))
		copy(updated, m.workflows)
		updated[idx].Status = status
		updated[idx].Detail = msg.Detail
		m.workflows = updated
	} else {
		// Append a new entry.
		entry := WorkflowEntry{
			ID:        id,
			Name:      msg.WorkflowName,
			Status:    status,
			StartedAt: msg.Timestamp,
			Detail:    msg.Detail,
		}
		if entry.Name == "" {
			entry.Name = id
		}

		// Copy the map to preserve value-receiver immutability.
		newIndex := make(map[string]int, len(m.workflowIndex)+1)
		for k, v := range m.workflowIndex {
			newIndex[k] = v
		}
		newIndex[id] = len(m.workflows)
		m.workflowIndex = newIndex

		m.workflows = append(m.workflows, entry)
	}

	return m
}

// handleKeyMsg processes navigation key events when the sidebar is focused.
func (m SidebarModel) handleKeyMsg(msg tea.KeyMsg) SidebarModel {
	n := len(m.workflows)
	if n == 0 {
		return m
	}

	switch msg.Type {
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "j":
			m.selectedIdx = clampIdx(m.selectedIdx+1, n)
		case "k":
			m.selectedIdx = clampIdx(m.selectedIdx-1, n)
		}
	case tea.KeyDown:
		m.selectedIdx = clampIdx(m.selectedIdx+1, n)
	case tea.KeyUp:
		m.selectedIdx = clampIdx(m.selectedIdx-1, n)
	}

	m.scrollOffset = adjustScroll(m.scrollOffset, m.selectedIdx, m.listHeight())
	return m
}

// clampIdx clamps idx to [0, n-1].
func clampIdx(idx, n int) int {
	if idx < 0 {
		return 0
	}
	if idx >= n {
		return n - 1
	}
	return idx
}

// adjustScroll ensures the selected row is visible in the scroll window.
// It returns the updated scroll offset.
func adjustScroll(offset, selected, visible int) int {
	if visible <= 0 {
		return 0
	}
	if selected < offset {
		return selected
	}
	if selected >= offset+visible {
		return selected - visible + 1
	}
	return offset
}

// ---------------------------------------------------------------------------
// View helpers
// ---------------------------------------------------------------------------

// listHeight returns the number of rows available for workflow entries inside
// the sidebar, accounting for the section header and separators.
//
// Layout within View():
//   row 0  : "WORKFLOWS" header
//   row 1  : (blank line after header produced by MarginBottom on SidebarTitle)
//   rows 2…: workflow entries (or "No workflows" placeholder)
//   …
//   remaining rows: future section placeholders + padding
//
// We reserve 2 rows for the header block (header text + its margin), then
// leave the rest for entries.
func (m SidebarModel) listHeight() int {
	const headerRows = 2 // header line + margin-bottom blank line
	h := m.height - headerRows
	if h < 0 {
		return 0
	}
	return h
}

// workflowIndicator returns a styled Unicode symbol for the given
// WorkflowStatus. Symbol mapping per task spec:
//
//	WorkflowRunning   → "●"  (theme.StatusRunning)
//	WorkflowIdle      → "○"  (theme.StatusBlocked — muted)
//	WorkflowPaused    → "◌"  (theme.StatusWaiting)
//	WorkflowCompleted → "✓"  (theme.StatusCompleted)
//	WorkflowFailed    → "✗"  (theme.StatusFailed)
func (m SidebarModel) workflowIndicator(status WorkflowStatus) string {
	switch status {
	case WorkflowRunning:
		return m.theme.StatusRunning.Render("●")
	case WorkflowPaused:
		return m.theme.StatusWaiting.Render("◌")
	case WorkflowCompleted:
		return m.theme.StatusCompleted.Render("✓")
	case WorkflowFailed:
		return m.theme.StatusFailed.Render("✗")
	default: // WorkflowIdle and unknown values
		return m.theme.StatusBlocked.Render("○")
	}
}

// truncateName truncates name to fit within maxWidth visible columns.
// If the name is wider it is shortened and an ellipsis "…" (1 column wide) is
// appended. If maxWidth <= 0 an empty string is returned.
func truncateName(name string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	w := lipgloss.Width(name)
	if w <= maxWidth {
		return name
	}
	// Walk runes until we consume maxWidth-1 columns (leave room for "…").
	target := maxWidth - 1
	var sb strings.Builder
	col := 0
	for _, r := range name {
		rw := lipgloss.Width(string(r))
		if col+rw > target {
			break
		}
		sb.WriteRune(r)
		col += rw
	}
	sb.WriteString("…")
	return sb.String()
}

// workflowListView renders the workflow list section (header + entries or
// placeholder). It does not apply the outer container style; that is handled
// by View().
func (m SidebarModel) workflowListView() string {
	var sb strings.Builder

	// Header.
	header := m.theme.SidebarTitle.Render("WORKFLOWS")
	sb.WriteString(header)
	sb.WriteString("\n")

	if len(m.workflows) == 0 {
		placeholder := m.theme.SidebarItem.Render("No workflows")
		sb.WriteString(placeholder)
		return sb.String()
	}

	// Determine visible slice via scroll window.
	visible := m.listHeight()
	if visible < 1 {
		visible = 1
	}

	start := m.scrollOffset
	end := start + visible
	if end > len(m.workflows) {
		end = len(m.workflows)
	}

	// Available width for the name:
	//   total width
	//   - 1 indicator column
	//   - 1 space between indicator and name
	// The SidebarItem style adds PaddingLeft(1), so actual rendered width
	// already includes that padding; we just need to account for the fixed
	// indicator+space prefix (2 columns) when calculating name truncation.
	nameWidth := m.width - 2 // indicator + space
	if nameWidth < 1 {
		nameWidth = 1
	}

	for i := start; i < end; i++ {
		entry := m.workflows[i]
		indicator := m.workflowIndicator(entry.Status)
		name := truncateName(entry.Name, nameWidth)
		line := indicator + " " + name

		if i == m.selectedIdx {
			if m.focused {
				sb.WriteString(m.theme.SidebarActive.Render(line))
			} else {
				sb.WriteString(m.theme.SidebarInactive.Render(line))
			}
		} else {
			sb.WriteString(m.theme.SidebarItem.Render(line))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the full sidebar panel as a string sized to the configured
// width and height. Sections are stacked vertically:
//
//  1. Workflow list  (this task)
//  2. Empty separator
//  3. Agent activity placeholder  (T-071)
//  4. Empty separator
//  5. Task progress placeholder   (T-072)
//  6. Padding rows to fill height
func (m SidebarModel) View() string {
	if m.width == 0 && m.height == 0 {
		return ""
	}

	var sb strings.Builder

	// Section 1: workflow list.
	sb.WriteString(m.workflowListView())
	sb.WriteString("\n")

	// Section 2: agent activity placeholder.
	agentHeader := m.theme.SidebarTitle.Render("AGENTS")
	sb.WriteString(agentHeader)
	sb.WriteString("\n")
	sb.WriteString(m.theme.SidebarItem.Render("(agent activity)"))
	sb.WriteString("\n")
	sb.WriteString("\n")

	// Section 3: task progress placeholder.
	progressHeader := m.theme.SidebarTitle.Render("PROGRESS")
	sb.WriteString(progressHeader)
	sb.WriteString("\n")
	sb.WriteString(m.theme.SidebarItem.Render("(task progress)"))
	sb.WriteString("\n")

	content := sb.String()

	// Count the lines already rendered so we can pad to full height.
	renderedLines := strings.Count(content, "\n")

	// Trim the trailing newline before padding so lipgloss does not add an
	// extra blank line at the top.
	content = strings.TrimRight(content, "\n")

	// Pad remaining rows with blank lines.
	remaining := m.height - renderedLines
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	}

	// Apply the outer container style (border + padding) if width > 0.
	// SidebarContainer has BorderRight(true), which adds 1 column. Subtract
	// it from Width() so the total rendered width equals m.width.
	if m.width > 0 {
		innerWidth := m.width - 1 // 1 for the right border character
		if innerWidth < 0 {
			innerWidth = 0
		}
		return m.theme.SidebarContainer.
			Width(innerWidth).
			Render(content)
	}

	return content
}
