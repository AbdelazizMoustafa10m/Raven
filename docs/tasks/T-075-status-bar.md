# T-075: Status Bar with Current State, Iteration, and Timer

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 4-6hrs |
| Dependencies | T-066, T-067, T-068, T-069 |
| Blocked By | T-069 |
| Blocks | T-078 |

## Goal
Implement the status bar at the bottom of the TUI that displays the current workflow phase, active task, iteration count, elapsed time, and quick-reference key hints. The status bar provides at-a-glance context about the current state of execution, updating in real-time as the workflow progresses.

## Background
Per PRD Section 5.12, the status bar renders as:
```
[Status Bar] Phase 2 | Task T-007 | Iter 3/20 | 00:14:32 | ? help
```

The status bar spans the full terminal width and uses a single line with contrasting background color (inverted from the main content area). Key information segments are separated by `|` dividers. The elapsed timer updates every second via `TickMsg`.

Status bar content comes from `LoopEventMsg` (iteration and task info), `WorkflowEventMsg` (phase and workflow info), and `TickMsg` (elapsed timer).

## Technical Specifications
### Implementation Approach
Create `internal/tui/status_bar.go` containing a `StatusBarModel` sub-model. It tracks the current phase, task, iteration/max iterations, workflow start time, and paused state. The view renders all fields in a single line with styled separators. The elapsed timer is computed from the start time on each `TickMsg`.

### Key Components
- **StatusBarModel**: Sub-model tracking status bar state
- **StatusSegment**: Individual key-value segment in the status bar
- **Elapsed timer**: Calculated from workflow start time, paused when workflow is paused

### API/Interface Contracts
```go
// internal/tui/status_bar.go

import (
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

// StatusBarModel manages the bottom status bar display.
type StatusBarModel struct {
    theme        Theme
    width        int

    // Dynamic state
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

// NewStatusBarModel creates a status bar with the given theme.
func NewStatusBarModel(theme Theme) StatusBarModel

// SetWidth updates the status bar width.
func (sb *StatusBarModel) SetWidth(width int)

// Update processes messages that affect status bar content.
func (sb StatusBarModel) Update(msg tea.Msg) StatusBarModel

// View renders the status bar as a single-line string.
func (sb StatusBarModel) View() string

// SetPaused updates the paused state (changes display to show "PAUSED").
func (sb *StatusBarModel) SetPaused(paused bool)

// formatElapsed converts a duration to "HH:MM:SS" format.
func formatElapsed(d time.Duration) string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/bubbletea | v1.2+ | tea.Msg |
| charmbracelet/lipgloss | v1.0+ | Status bar styling |
| internal/tui (T-067) | - | LoopEventMsg, WorkflowEventMsg, TickMsg |
| internal/tui (T-068) | - | Theme (StatusBar, StatusKey, StatusValue styles) |

## Acceptance Criteria
- [ ] Status bar renders as a single line at full terminal width
- [ ] Status bar has a contrasting background color (inverted theme)
- [ ] Phase segment displays "Phase N/M" or "Phase N" if total is unknown
- [ ] Task segment displays current task ID (e.g., "T-007") or "--" if none
- [ ] Iteration segment displays "Iter N/M" (e.g., "Iter 3/20")
- [ ] Elapsed timer displays "HH:MM:SS" format, updating every second
- [ ] Key hint segment shows "? help" at the right edge
- [ ] Segments are separated by styled " | " dividers
- [ ] When paused, a "PAUSED" indicator appears prominently (e.g., yellow background)
- [ ] `LoopEventMsg` with task and iteration info updates corresponding segments
- [ ] `WorkflowEventMsg` updates phase and workflow name
- [ ] `TickMsg` updates elapsed time
- [ ] Status bar truncates gracefully if terminal is too narrow (hide rightmost segments first)
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- `NewStatusBarModel` creates model with default "idle" state
- `Update` with `LoopEventMsg{TaskID: "T-007", Iteration: 3, MaxIter: 20}` sets task and iteration
- `Update` with `WorkflowEventMsg` sets phase and workflow name
- `Update` with `TickMsg` updates elapsed time
- `View()` at width 100 contains all segments: phase, task, iteration, elapsed, help hint
- `View()` at width 40 truncates gracefully (drops rightmost segments)
- `SetPaused(true)` adds "PAUSED" to the view
- `SetPaused(false)` removes "PAUSED"
- `formatElapsed(0)` returns "00:00:00"
- `formatElapsed(90 * time.Second)` returns "00:01:30"
- `formatElapsed(3661 * time.Second)` returns "01:01:01"
- Elapsed timer pauses when `paused=true`

### Integration Tests
- Simulate a workflow lifecycle: start, iterate, pause, resume, complete -- verify status bar at each stage

### Edge Cases to Handle
- No active workflow (show "idle" mode)
- Iteration count of 0/0 (show "Iter --")
- Very long task ID or phase name (truncate to fit)
- Terminal width exactly matching minimum (80) -- verify segments fit
- Timer running for 24+ hours (format still works with large hour values)

## Implementation Notes
### Recommended Approach
1. Define `StatusBarModel` struct with all tracked fields
2. Implement `Update` with type switch:
   - `LoopEventMsg`: extract task, iteration, maxIteration based on event type
   - `WorkflowEventMsg`: extract workflow name, derive phase from step info
   - `TickMsg`: if !paused, calculate `elapsed = time.Since(startTime)`
3. Implement `View()`:
   - Build segments: `[mode] | Phase {phase} | Task {task} | Iter {iter}/{max} | {elapsed}`
   - Build right-aligned hint: `? help`
   - Calculate total width; if exceeds available, drop segments from right to left (keep mode and task)
   - Apply theme.StatusBar style with full width
   - Use lipgloss to render left-aligned segments + right-aligned hint
4. Implement `formatElapsed`:
   - `hours := int(d.Hours())`
   - `mins := int(d.Minutes()) % 60`
   - `secs := int(d.Seconds()) % 60`
   - `fmt.Sprintf("%02d:%02d:%02d", hours, mins, secs)`
5. Implement `SetPaused`: store flag, display "PAUSED" in warning color

### Potential Pitfalls
- Right-aligning the help hint while left-aligning the segments requires calculating available space: `remainingWidth = width - lipgloss.Width(leftSegments) - lipgloss.Width(rightHint)`. Fill with spaces or use `lipgloss.Place`.
- The status bar must be exactly 1 line and exactly `width` characters. Use `lipgloss.NewStyle().Width(width).MaxHeight(1)` to enforce this.
- Elapsed timer updates should use `time.Since(startTime)` not an incremented counter, to account for tick drift.
- Do not process key messages in the status bar -- it is display-only.

### Security Considerations
- No security considerations for status bar display

## References
- [Lipgloss Place for Alignment](https://pkg.go.dev/github.com/charmbracelet/lipgloss#Place)
- [PRD Section 5.12 - Status Bar Specification](docs/prd/PRD-Raven.md)