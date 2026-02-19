package tui

import (
	"fmt"
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
// TaskProgressSection
// ---------------------------------------------------------------------------

// TaskProgressSection tracks task and phase completion for the sidebar.
// It is a value type; all mutations return a new copy, consistent with the
// Bubble Tea Elm-architecture pattern used throughout the TUI package.
type TaskProgressSection struct {
	theme Theme

	// Overall task progress across all phases.
	totalTasks     int
	completedTasks int

	// Phase progress.
	currentPhase   int
	totalPhases    int
	phaseTasks     int // Total tasks in the current phase.
	phaseCompleted int // Completed tasks in the current phase.
}

// NewTaskProgressSection creates a TaskProgressSection with the given theme
// and zero-initialised counters.
func NewTaskProgressSection(theme Theme) TaskProgressSection {
	return TaskProgressSection{theme: theme}
}

// SetTotals initialises the overall task count and the total number of phases.
// Negative values are treated as zero.
func (tp *TaskProgressSection) SetTotals(totalTasks, totalPhases int) {
	if totalTasks < 0 {
		totalTasks = 0
	}
	if totalPhases < 0 {
		totalPhases = 0
	}
	tp.totalTasks = totalTasks
	tp.totalPhases = totalPhases
}

// SetPhase updates the current phase number and the task counts for that phase.
// Negative values are treated as zero.
func (tp *TaskProgressSection) SetPhase(phase, phaseTasks, phaseCompleted int) {
	if phase < 0 {
		phase = 0
	}
	if phaseTasks < 0 {
		phaseTasks = 0
	}
	if phaseCompleted < 0 {
		phaseCompleted = 0
	}
	tp.currentPhase = phase
	tp.phaseTasks = phaseTasks
	tp.phaseCompleted = phaseCompleted
}

// Update processes tea.Msg values relevant to task progress and returns the
// updated section. Handled messages:
//   - TaskProgressMsg     — updates overall completedTasks / totalTasks from
//     the message's Completed / Total fields.
//   - LoopEventMsg        — LoopPhaseComplete increments currentPhase and
//     resets phaseCompleted; LoopTaskCompleted increments phaseCompleted.
func (tp TaskProgressSection) Update(msg tea.Msg) TaskProgressSection {
	switch msg := msg.(type) {
	case TaskProgressMsg:
		completed := msg.Completed
		total := msg.Total
		if completed < 0 {
			completed = 0
		}
		if total < 0 {
			total = 0
		}
		// Clamp completed to total.
		if completed > total {
			completed = total
		}
		tp.completedTasks = completed
		tp.totalTasks = total

	case LoopEventMsg:
		switch msg.Type {
		case LoopPhaseComplete:
			tp.currentPhase++
			tp.phaseCompleted = 0
		case LoopTaskCompleted:
			tp.phaseCompleted++
			// Also increment the overall completed count to stay in sync when
			// the overall total has been set via SetTotals but no TaskProgressMsg
			// has arrived yet.
			if tp.completedTasks < tp.totalTasks {
				tp.completedTasks++
			}
		default:
		}
	}

	return tp
}

// View renders the task progress section as a string constrained to width
// columns. It renders two sub-sections:
//
//  1. Overall task progress  (header "Tasks", bar, percentage, "N/M done")
//  2. Phase progress          (header "Phase: N/M", bar, percentage)
//
// When total counts are zero the respective sub-section shows a "No tasks" /
// "No phases" placeholder instead of a progress bar.
func (tp TaskProgressSection) View(width int) string {
	var sb strings.Builder

	// --- Overall task progress ---
	sb.WriteString(tp.theme.SidebarTitle.Render("Tasks"))
	sb.WriteString("\n")

	if tp.totalTasks == 0 {
		sb.WriteString(tp.theme.SidebarItem.Render("No tasks"))
		sb.WriteString("\n")
	} else {
		completed := tp.completedTasks
		if completed > tp.totalTasks {
			completed = tp.totalTasks
		}
		fraction := float64(completed) / float64(tp.totalTasks)

		barWidth := width - 2 // 1-char padding each side
		if barWidth < 1 {
			barWidth = 1
		}

		sb.WriteString(tp.theme.ProgressBar(fraction, barWidth))
		sb.WriteString("\n")
		sb.WriteString(tp.theme.ProgressPercent.Render(fmt.Sprintf("%d%%", int(fraction*100))))
		sb.WriteString("\n")
		sb.WriteString(tp.theme.ProgressLabel.Render(fmt.Sprintf("%d/%d done", completed, tp.totalTasks)))
		sb.WriteString("\n")
	}

	// Blank line between sub-sections.
	sb.WriteString("\n")

	// --- Phase progress ---
	phaseHeader := fmt.Sprintf("Phase: %d/%d", tp.currentPhase, tp.totalPhases)
	sb.WriteString(tp.theme.SidebarTitle.Render(phaseHeader))
	sb.WriteString("\n")

	if tp.totalPhases == 0 {
		sb.WriteString(tp.theme.SidebarItem.Render("No phases"))
		sb.WriteString("\n")
	} else {
		phaseCompleted := tp.phaseCompleted
		phaseTasks := tp.phaseTasks
		if phaseTasks < 0 {
			phaseTasks = 0
		}
		if phaseCompleted < 0 {
			phaseCompleted = 0
		}
		if phaseTasks > 0 && phaseCompleted > phaseTasks {
			phaseCompleted = phaseTasks
		}

		var phaseFraction float64
		if phaseTasks > 0 {
			phaseFraction = float64(phaseCompleted) / float64(phaseTasks)
		}

		barWidth := width - 2
		if barWidth < 1 {
			barWidth = 1
		}

		sb.WriteString(tp.theme.ProgressBar(phaseFraction, barWidth))
		sb.WriteString("\n")
		sb.WriteString(tp.theme.ProgressPercent.Render(fmt.Sprintf("%d%%", int(phaseFraction*100))))
		sb.WriteString("\n")
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// ProviderRateLimit
// ---------------------------------------------------------------------------

// ProviderRateLimit tracks the rate-limit state for a single provider.
// It is a value type used inside RateLimitSection.
type ProviderRateLimit struct {
	// Provider is the AI provider name (e.g. "anthropic", "openai").
	Provider string
	// Agent is the agent name that hit the rate limit (e.g. "claude").
	Agent string
	// ResetAt is the absolute time at which the rate limit is expected to clear.
	ResetAt time.Time
	// Remaining is the time left until the rate limit clears, recalculated on
	// each TickMsg using time.Until(ResetAt).
	Remaining time.Duration
	// Active is true while the countdown is running (Remaining > 0).
	Active bool
}

// ---------------------------------------------------------------------------
// RateLimitSection
// ---------------------------------------------------------------------------

// RateLimitSection renders the rate-limit status display in the sidebar.
// It tracks per-provider state and drives a per-second countdown timer via
// TickCmd. It is a value type consistent with Bubble Tea's Elm architecture.
type RateLimitSection struct {
	theme Theme
	// providers maps provider name → rate-limit state.
	providers map[string]*ProviderRateLimit
	// order holds provider names in stable insertion order for rendering.
	order []string
}

// NewRateLimitSection creates a RateLimitSection initialised with the given
// theme and an empty provider map.
func NewRateLimitSection(theme Theme) RateLimitSection {
	return RateLimitSection{
		theme:     theme,
		providers: make(map[string]*ProviderRateLimit),
	}
}

// Update handles RateLimitMsg and TickMsg messages and returns the updated
// section together with a follow-up command.
//
//   - RateLimitMsg: registers or updates the named provider's reset time, marks
//     it Active, and returns TickCmd(time.Second) to start the countdown.
//   - TickMsg: recalculates Remaining = time.Until(ResetAt) for every provider
//     and clears Active when Remaining has reached zero. Returns TickCmd if any
//     provider is still active; nil otherwise.
func (rl RateLimitSection) Update(msg tea.Msg) (RateLimitSection, tea.Cmd) {
	switch msg := msg.(type) {
	case RateLimitMsg:
		rl = rl.applyRateLimitMsg(msg)
		return rl, TickCmd(time.Second)

	case TickMsg:
		_ = msg // tick time not needed; Remaining is recalculated via time.Until(ResetAt)
		rl = rl.tick()
		if rl.HasActiveLimit() {
			return rl, TickCmd(time.Second)
		}
		return rl, nil
	}

	return rl, nil
}

// applyRateLimitMsg updates (or inserts) the provider entry from a RateLimitMsg.
// It copies the providers map and order slice to honour value-receiver semantics.
func (rl RateLimitSection) applyRateLimitMsg(msg RateLimitMsg) RateLimitSection {
	key := msg.Provider
	if key == "" {
		key = msg.Agent
	}

	// Determine ResetAt: prefer the explicit ResetAt if non-zero; otherwise
	// derive from ResetAfter relative to the message timestamp.
	resetAt := msg.ResetAt
	if resetAt.IsZero() {
		ts := msg.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}
		resetAt = ts.Add(msg.ResetAfter)
	}

	remaining := time.Until(resetAt)
	if remaining < 0 {
		remaining = 0
	}

	// Copy providers map for immutability.
	newProviders := make(map[string]*ProviderRateLimit, len(rl.providers))
	for k, v := range rl.providers {
		cp := *v
		newProviders[k] = &cp
	}

	newOrder := rl.order
	if _, exists := newProviders[key]; !exists {
		// Append to order only for new providers; copy the slice first.
		newOrder = make([]string, len(rl.order)+1)
		copy(newOrder, rl.order)
		newOrder[len(rl.order)] = key
	}

	newProviders[key] = &ProviderRateLimit{
		Provider:  msg.Provider,
		Agent:     msg.Agent,
		ResetAt:   resetAt,
		Remaining: remaining,
		Active:    true,
	}

	rl.providers = newProviders
	rl.order = newOrder
	return rl
}

// tick recalculates Remaining for every provider and deactivates expired ones.
func (rl RateLimitSection) tick() RateLimitSection {
	if len(rl.providers) == 0 {
		return rl
	}

	newProviders := make(map[string]*ProviderRateLimit, len(rl.providers))
	for k, v := range rl.providers {
		cp := *v
		if cp.Active {
			cp.Remaining = time.Until(cp.ResetAt)
			if cp.Remaining <= 0 {
				cp.Remaining = 0
				cp.Active = false
			}
		}
		newProviders[k] = &cp
	}

	rl.providers = newProviders
	return rl
}

// HasActiveLimit returns true when at least one provider currently has Active == true.
func (rl RateLimitSection) HasActiveLimit() bool {
	for _, prl := range rl.providers {
		if prl.Active {
			return true
		}
	}
	return false
}

// View renders the "Rate Limits" section header followed by one line per known
// provider. Lines are truncated to fit within width columns.
//
// Format per provider:
//   - No active limit: "{name}: OK"
//   - Active limit:    "{name}: WAIT M:SS"
//
// When no providers are known, a placeholder "No limits" line is shown instead.
func (rl RateLimitSection) View(width int) string {
	var sb strings.Builder

	sb.WriteString(rl.theme.SidebarTitle.Render("Rate Limits"))
	sb.WriteString("\n")

	if len(rl.order) == 0 {
		sb.WriteString(rl.theme.SidebarItem.Render("No limits"))
		sb.WriteString("\n")
		return sb.String()
	}

	for _, key := range rl.order {
		prl, ok := rl.providers[key]
		if !ok {
			continue
		}

		name := prl.Provider
		if name == "" {
			name = prl.Agent
		}
		if name == "" {
			name = key
		}

		var line string
		if prl.Active {
			countdown := formatCountdown(prl.Remaining)
			suffix := ": " + rl.theme.StatusWaiting.Render("WAIT "+countdown)
			if width > 0 {
				// Reserve width for the suffix before truncating the name.
				suffixWidth := lipgloss.Width(": WAIT " + countdown)
				nameAllowed := width - suffixWidth
				if nameAllowed < 1 {
					nameAllowed = 1
				}
				line = truncateName(name, nameAllowed) + suffix
			} else {
				line = name + suffix
			}
		} else {
			suffix := ": " + rl.theme.StatusCompleted.Render("OK")
			if width > 0 {
				suffixWidth := lipgloss.Width(": OK")
				nameAllowed := width - suffixWidth
				if nameAllowed < 1 {
					nameAllowed = 1
				}
				line = truncateName(name, nameAllowed) + suffix
			} else {
				line = name + suffix
			}
		}

		sb.WriteString(rl.theme.SidebarItem.Render(line))
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatCountdown formats a duration as "M:SS" (under 1 hour) or "H:MM:SS"
// (1 hour or more). Negative durations return "0:00".
func formatCountdown(d time.Duration) string {
	if d <= 0 {
		return "0:00"
	}

	totalSec := int(d.Seconds())
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60

	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// ---------------------------------------------------------------------------
// SidebarModel
// ---------------------------------------------------------------------------

// SidebarModel is the Bubble Tea sub-model for the sidebar panel.
// It maintains the workflow list section (T-070), the task progress section
// (T-071), and the rate-limit status section (T-072).
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

	// taskProgress tracks overall and per-phase task completion.
	taskProgress TaskProgressSection

	// rateLimits holds the per-provider rate-limit countdown display (T-072).
	rateLimits RateLimitSection
}

// NewSidebarModel creates a SidebarModel with the given theme and an empty
// workflow list. Dimensions default to zero until SetDimensions is called.
func NewSidebarModel(theme Theme) SidebarModel {
	return SidebarModel{
		theme:         theme,
		workflowIndex: make(map[string]int),
		taskProgress:  NewTaskProgressSection(theme),
		rateLimits:    NewRateLimitSection(theme),
	}
}

// SetTotals initialises the overall task count and total phase count shown in
// the task progress section. It delegates to TaskProgressSection.SetTotals.
func (m *SidebarModel) SetTotals(totalTasks, totalPhases int) {
	m.taskProgress.SetTotals(totalTasks, totalPhases)
}

// SetPhase updates the current phase number and its task counts in the task
// progress section. It delegates to TaskProgressSection.SetPhase.
func (m *SidebarModel) SetPhase(phase, phaseTasks, phaseCompleted int) {
	m.taskProgress.SetPhase(phase, phaseTasks, phaseCompleted)
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
//   - TaskProgressMsg   — updates overall task completion counters
//   - LoopEventMsg      — updates phase and per-phase task counters
//   - RateLimitMsg      — registers or updates a provider rate-limit countdown
//   - TickMsg           — advances the rate-limit countdown timers
//   - FocusChangedMsg   — updates the focused flag
//   - tea.KeyMsg        — j/k/up/down navigation when focused
func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	switch msg := msg.(type) {
	case WorkflowEventMsg:
		m = m.handleWorkflowEvent(msg)

	case TaskProgressMsg:
		m.taskProgress = m.taskProgress.Update(msg)

	case LoopEventMsg:
		m.taskProgress = m.taskProgress.Update(msg)

	case RateLimitMsg:
		var cmd tea.Cmd
		m.rateLimits, cmd = m.rateLimits.Update(msg)
		return m, cmd

	case TickMsg:
		var cmd tea.Cmd
		m.rateLimits, cmd = m.rateLimits.Update(msg)
		return m, cmd

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
	default:
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
//
//	row 0  : "WORKFLOWS" header
//	row 1  : (blank line after header produced by MarginBottom on SidebarTitle)
//	rows 2…: workflow entries (or "No workflows" placeholder)
//	…
//	remaining rows: future section placeholders + padding
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
//  1. Workflow list     (T-070)
//  2. Separator
//  3. Agent activity    (placeholder; T-073)
//  4. Separator
//  5. Rate limits       (T-072)
//  6. Separator
//  7. Task progress     (T-071)
//  8. Padding rows to fill height
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

	// Section 3: rate limits.
	sb.WriteString(m.rateLimits.View(m.width))
	sb.WriteString("\n")

	// Section 4: task progress.
	progressHeader := m.theme.SidebarTitle.Render("PROGRESS")
	sb.WriteString(progressHeader)
	sb.WriteString("\n")
	sb.WriteString(m.taskProgress.View(m.width))
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
