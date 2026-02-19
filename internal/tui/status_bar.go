package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusBarModel manages the bottom status bar display in the Raven TUI.
// It tracks the current workflow phase, active task, iteration count,
// elapsed time, and paused state. The view renders all fields in a single
// line with styled separators. The elapsed timer is computed from the start
// time on each TickMsg.
//
// StatusBarModel follows Bubble Tea's Elm architecture: Update returns a new
// value, and View is a pure function of the model state.
type StatusBarModel struct {
	theme Theme
	width int

	// Dynamic state updated by incoming messages.
	phase        string // e.g., "Phase 2/5" or "Phase 2"
	task         string // e.g., "T-007"
	iteration    int
	maxIteration int
	startTime    time.Time
	elapsed      time.Duration
	paused       bool
	workflow     string // e.g., "pipeline"
	mode         string // e.g., "implement", "review", "idle"
}

// NewStatusBarModel creates a StatusBarModel with the given theme.
// All dynamic state fields start at their zero values; the mode defaults to
// "idle" and no start time is set until a message initialises it.
func NewStatusBarModel(theme Theme) StatusBarModel {
	return StatusBarModel{
		theme: theme,
		mode:  "idle",
	}
}

// SetWidth updates the status bar width. This should be called whenever the
// parent App processes a tea.WindowSizeMsg.
func (sb *StatusBarModel) SetWidth(width int) {
	sb.width = width
}

// SetPaused updates the paused state. When true, the status bar displays a
// prominent "PAUSED" indicator in warning colour instead of the elapsed timer.
func (sb *StatusBarModel) SetPaused(paused bool) {
	sb.paused = paused
}

// Update processes messages that affect status bar content and returns the
// updated model.
//
// Handled messages:
//   - LoopEventMsg     — updates task, iteration, maxIteration, mode, and
//     initialises the start time on the first LoopIterationStarted event.
//   - WorkflowEventMsg — updates workflow name and phase derived from Step.
//   - TickMsg          — advances the elapsed timer when not paused.
func (sb StatusBarModel) Update(msg tea.Msg) StatusBarModel {
	switch m := msg.(type) {
	case LoopEventMsg:
		sb = sb.handleLoopEvent(m)

	case WorkflowEventMsg:
		sb = sb.handleWorkflowEvent(m)

	case TickMsg:
		if !sb.paused && !sb.startTime.IsZero() {
			elapsed := m.Time.Sub(sb.startTime)
			if elapsed < 0 {
				elapsed = 0
			}
			sb.elapsed = elapsed
		}
	}

	return sb
}

// handleLoopEvent extracts task, iteration, and mode information from a
// LoopEventMsg and updates the model accordingly.
func (sb StatusBarModel) handleLoopEvent(msg LoopEventMsg) StatusBarModel {
	switch msg.Type {
	case LoopIterationStarted:
		// Initialise the start time once, on the very first iteration.
		if sb.startTime.IsZero() {
			if !msg.Timestamp.IsZero() {
				sb.startTime = msg.Timestamp
			} else {
				sb.startTime = time.Now()
			}
		}
		if msg.Iteration > 0 {
			sb.iteration = msg.Iteration
		}
		if msg.MaxIter > 0 {
			sb.maxIteration = msg.MaxIter
		}
		sb.mode = "implement"

	case LoopIterationCompleted:
		if msg.Iteration > 0 {
			sb.iteration = msg.Iteration
		}
		if msg.MaxIter > 0 {
			sb.maxIteration = msg.MaxIter
		}

	case LoopTaskSelected:
		if msg.TaskID != "" {
			sb.task = msg.TaskID
		}
		if msg.Iteration > 0 {
			sb.iteration = msg.Iteration
		}
		if msg.MaxIter > 0 {
			sb.maxIteration = msg.MaxIter
		}

	case LoopTaskCompleted:
		if msg.TaskID != "" {
			sb.task = msg.TaskID
		}

	case LoopTaskBlocked:
		if msg.TaskID != "" {
			sb.task = msg.TaskID
		}

	case LoopWaitingForRateLimit:
		sb.paused = true

	case LoopResumedAfterWait:
		sb.paused = false

	case LoopPhaseComplete:
		sb.mode = "idle"

	default:
	}

	return sb
}

// handleWorkflowEvent extracts workflow name and phase information from a
// WorkflowEventMsg and updates the model accordingly.
func (sb StatusBarModel) handleWorkflowEvent(msg WorkflowEventMsg) StatusBarModel {
	if msg.WorkflowName != "" {
		sb.workflow = msg.WorkflowName
	}

	if msg.Step != "" {
		sb.phase = msg.Step
	}

	// Derive mode from the event type when available.
	switch strings.ToLower(msg.Event) {
	case "idle", "stopped", "not_started":
		sb.mode = "idle"
	case "completed", "done", "success":
		sb.mode = "done"
	case "failed", "error":
		sb.mode = "error"
	case "paused", "waiting", "rate_limited":
		sb.paused = true
	default:
		if sb.mode == "idle" {
			sb.mode = "running"
		}
	}

	return sb
}

// View renders the status bar as a single-line string spanning the full
// terminal width. Segments are left-aligned, separated by styled dividers.
// A "? help" hint is right-aligned. If the total segment width exceeds the
// available width, rightmost optional segments are omitted to ensure the bar
// fits exactly in one line.
//
// Rendered format (approximate):
//
//	[mode] | Phase {phase} | Task {task} | Iter {n}/{max} | {elapsed} | ? help
func (sb StatusBarModel) View() string {
	if sb.width <= 0 {
		return ""
	}

	sep := sb.theme.StatusSeparator.Render(" | ")

	// --- Build individual segment strings ---

	modeStr := sb.modeSegment()
	phaseStr := sb.phaseSegment()
	taskStr := sb.taskSegment()
	iterStr := sb.iterSegment()
	timerStr := sb.timerSegment()
	helpStr := sb.theme.HelpKey.Render("?") + " " + sb.theme.HelpDesc.Render("help")

	// Mandatory segments (always shown if they fit): mode + task.
	// Optional segments (hidden first when narrow): iter, timer, phase.
	type segment struct {
		text     string
		optional bool
	}

	segments := []segment{
		{text: modeStr, optional: false},
		{text: sep + phaseStr, optional: true},
		{text: sep + taskStr, optional: false},
		{text: sep + iterStr, optional: true},
		{text: sep + timerStr, optional: true},
	}

	// StatusBar theme style has Padding(0,1), i.e. 1 column on each side = 2
	// total columns consumed by padding. We pass Width(innerWidth) to lipgloss
	// so it pads the content to innerWidth and then adds the 1+1 = 2 padding
	// columns, giving a total rendered width of sb.width.
	const barPadding = 2
	innerWidth := sb.width - barPadding
	if innerWidth < 0 {
		innerWidth = 0
	}

	// Reserve space inside innerWidth for the right-aligned help hint
	// (including its leading separator).
	helpSepStr := sep + helpStr
	helpSegWidth := lipgloss.Width(helpSepStr)

	// Compute mandatory-only width to know how much optional budget we have.
	mandatoryWidth := 0
	for _, seg := range segments {
		if !seg.optional {
			mandatoryWidth += lipgloss.Width(seg.text)
		}
	}

	// Budget available for optional segments (between mandatory content and help hint).
	optionalBudget := innerWidth - mandatoryWidth - helpSegWidth
	if optionalBudget < 0 {
		optionalBudget = 0
	}

	// Build the ordered segment list: always include mandatory segments,
	// greedily include optional segments while they fit within optionalBudget.
	var leftParts []string
	optionalUsed := 0

	for _, seg := range segments {
		w := lipgloss.Width(seg.text)
		if !seg.optional {
			// Mandatory: always include.
			leftParts = append(leftParts, seg.text)
		} else if optionalUsed+w <= optionalBudget {
			// Optional: include only if it fits within the optional budget.
			leftParts = append(leftParts, seg.text)
			optionalUsed += w
		}
		// Optional segments that exceed the budget are skipped.
	}

	leftContent := strings.Join(leftParts, "")

	// Fill the gap between the left content and the right-aligned hint.
	leftWidth := lipgloss.Width(leftContent)
	gap := innerWidth - leftWidth - helpSegWidth
	if gap < 0 {
		gap = 0
	}
	padding := strings.Repeat(" ", gap)

	// Compose full bar content.
	barContent := leftContent + padding + helpSepStr

	// Apply the StatusBar style. Width(sb.width) sets the total rendered width
	// (lipgloss uses the border-box model where Width includes padding).
	// With Padding(0,1) the content area is sb.width-2, which matches innerWidth.
	// MaxHeight(1) ensures no line wrapping.
	return sb.theme.StatusBar.
		Width(sb.width).
		MaxHeight(1).
		Render(barContent)
}

// modeSegment returns the styled mode label (e.g., "[implement]" or "[idle]").
// When paused it returns a prominent "PAUSED" indicator.
func (sb StatusBarModel) modeSegment() string {
	if sb.paused {
		pausedStyle := lipgloss.NewStyle().
			Bold(true).
			Background(ColorWarning).
			Foreground(lipgloss.Color("#000000")).
			Padding(0, 1)
		return pausedStyle.Render("PAUSED")
	}

	label := sb.mode
	if label == "" {
		label = "idle"
	}
	return sb.theme.StatusKey.Render("[" + label + "]")
}

// phaseSegment returns the styled phase label.
// Returns "Phase --" when no phase information is available.
func (sb StatusBarModel) phaseSegment() string {
	phase := sb.phase
	if phase == "" {
		phase = "--"
	}
	return sb.theme.StatusKey.Render("Phase") + " " + sb.theme.StatusValue.Render(phase)
}

// taskSegment returns the styled task label.
// Returns "Task --" when no task has been set.
func (sb StatusBarModel) taskSegment() string {
	task := sb.task
	if task == "" {
		task = "--"
	}
	return sb.theme.StatusKey.Render("Task") + " " + sb.theme.StatusValue.Render(task)
}

// iterSegment returns the styled iteration counter.
// Returns "Iter 0/0" when neither field has been set.
func (sb StatusBarModel) iterSegment() string {
	iter := sb.theme.StatusValue.Render(
		fmt.Sprintf("%d/%d", sb.iteration, sb.maxIteration),
	)
	return sb.theme.StatusKey.Render("Iter") + " " + iter
}

// timerSegment returns the styled elapsed time in HH:MM:SS format.
// When paused, the elapsed time is frozen at its last known value.
func (sb StatusBarModel) timerSegment() string {
	return sb.theme.StatusKey.Render("Time") + " " +
		sb.theme.StatusValue.Render(formatElapsed(sb.elapsed))
}

// formatElapsed converts a duration to "HH:MM:SS" format.
// Negative durations are treated as zero.
func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, mins, secs)
}
