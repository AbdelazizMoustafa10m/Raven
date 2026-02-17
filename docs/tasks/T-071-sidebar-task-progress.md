# T-071: Sidebar -- Task Progress Bars and Phase Progress

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-8hrs |
| Dependencies | T-067, T-068, T-069, T-070 |
| Blocked By | T-070 |
| Blocks | T-078 |

## Goal
Implement the task progress and phase progress sections of the sidebar panel, displaying real-time progress bars for overall task completion and per-phase progress. The progress bars update in response to `TaskProgressMsg` messages as tasks are completed during workflow execution.

## Background
Per PRD Section 5.12, the sidebar displays:
```
Tasks
████░░ 40%
12/30 done

Phase: 2/5
██░░░░ 28%
```

This shows two progress sections: (1) overall task progress across all phases, and (2) the current phase's progress. Both use text-based progress bars rendered within the sidebar's fixed width. The progress bars use filled/empty block characters styled with the theme's progress colors.

Progress data comes from `TaskProgressMsg` messages (T-067) which carry the task status, completed count, and total count. Phase information comes from the workflow engine through `LoopEventMsg`.

## Technical Specifications
### Implementation Approach
Add a `TaskProgressSection` to the sidebar model in `internal/tui/sidebar.go` (extending the file created in T-070). This section tracks overall completion (completed/total tasks), current phase number, total phases, and per-phase completion. It renders progress bars using the `Theme.ProgressBar` helper and formats completion counts.

### Key Components
- **TaskProgressSection**: State for task and phase progress display
- **PhaseProgress**: Tracks per-phase completion (phase number, completed, total)
- **Progress bar rendering**: Uses Theme.ProgressBar (T-068) constrained to sidebar width

### API/Interface Contracts
```go
// internal/tui/sidebar.go (extension)

// TaskProgressSection tracks task and phase completion for the sidebar.
type TaskProgressSection struct {
    theme        Theme

    // Overall task progress
    totalTasks     int
    completedTasks int

    // Phase progress
    currentPhase   int
    totalPhases    int
    phaseTasks     int    // Tasks in current phase
    phaseCompleted int    // Completed tasks in current phase
}

// NewTaskProgressSection creates a progress section with the given theme.
func NewTaskProgressSection(theme Theme) TaskProgressSection

// Update processes TaskProgressMsg and LoopEventMsg to update progress state.
func (tp TaskProgressSection) Update(msg tea.Msg) TaskProgressSection

// View renders the task progress section as a string.
// The width parameter constrains the rendered output to the sidebar width.
func (tp TaskProgressSection) View(width int) string

// SetTotals initializes the total task and phase counts.
func (tp *TaskProgressSection) SetTotals(totalTasks, totalPhases int)

// SetPhase updates the current phase information.
func (tp *TaskProgressSection) SetPhase(phase, phaseTasks, phaseCompleted int)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/lipgloss | v1.0+ | Styling progress bars and labels |
| internal/tui (T-067) | - | TaskProgressMsg, LoopEventMsg |
| internal/tui (T-068) | - | Theme, ProgressBar helper |

## Acceptance Criteria
- [ ] "Tasks" section header displayed above the task progress bar
- [ ] Overall task progress bar renders at the correct width (sidebar width minus padding)
- [ ] Progress bar shows filled/empty proportions matching `completedTasks/totalTasks`
- [ ] Completion count text shows "N/M done" format (e.g., "12/30 done")
- [ ] Percentage is displayed next to or above the progress bar
- [ ] "Phase: N/M" header displayed above the phase progress bar
- [ ] Phase progress bar shows correct completion for the current phase
- [ ] `TaskProgressMsg` updates the completion counts
- [ ] `LoopEventMsg` with `LoopPhaseComplete` increments current phase
- [ ] Zero total tasks shows "No tasks" instead of a progress bar
- [ ] Progress never exceeds 100% (completedTasks clamped to totalTasks)
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- `NewTaskProgressSection` creates section with zero counts
- `SetTotals(30, 5)` sets totalTasks=30, totalPhases=5
- `Update` with `TaskProgressMsg{Completed: 12, Total: 30}` updates counts
- `Update` with `TaskProgressMsg{Status: "completed"}` increments completed count
- `View(20)` with 0/30 tasks shows empty progress bar
- `View(20)` with 15/30 tasks shows half-filled progress bar
- `View(20)` with 30/30 tasks shows full progress bar
- `View(20)` with zero total tasks shows "No tasks" message
- `SetPhase(2, 10, 4)` updates phase display correctly
- Phase progress bar updates independently of overall progress
- Completion text shows correct "N/M done" format

### Integration Tests
- Send sequence of TaskProgressMsg messages simulating task completions and verify view updates

### Edge Cases to Handle
- Total tasks is zero (avoid division by zero in percentage calculation)
- Completed exceeds total (clamp to total)
- Very large task count (1000+ tasks) -- numbers should not overflow sidebar width
- Phase change during display (phase counter resets phase progress)
- Negative values in messages (treat as zero)

## Implementation Notes
### Recommended Approach
1. Define `TaskProgressSection` struct with theme, total/completed counts, and phase info
2. Implement `Update` with type switch:
   - `TaskProgressMsg`: update `completedTasks` and `totalTasks` from message fields
   - `LoopEventMsg` with `LoopPhaseComplete`: increment `currentPhase`, reset phase counters
   - `LoopEventMsg` with `LoopTaskCompleted`: increment `phaseCompleted`
3. Implement `View(width int)`:
   - Render "Tasks" header
   - Calculate bar width: `width - 2` (1 char padding each side)
   - Calculate filled fraction: `float64(completedTasks) / float64(totalTasks)`
   - Render progress bar using `theme.ProgressBar(fraction, barWidth)`
   - Render completion text: `fmt.Sprintf("%d/%d done", completed, total)`
   - Add blank line
   - Render "Phase: N/M" header
   - Render phase progress bar similarly
4. Integrate into `SidebarModel.View()` between workflow list and rate limit sections

### Potential Pitfalls
- Division by zero: always check `totalTasks > 0` before computing percentage
- The progress bar width must account for the sidebar's internal padding and border characters. Use `lipgloss.Width()` to measure rendered strings, not byte length.
- Progress bar characters may have different visual widths in some terminals. Test with monospace-only characters.
- When integrating into `SidebarModel`, allocate specific line counts for each section to prevent sections from overlapping.

### Security Considerations
- No security considerations for progress display

## References
- [Bubbles Progress Component](https://github.com/charmbracelet/bubbles/tree/master/progress)
- [PRD Section 5.12 - Sidebar Task Progress](docs/prd/PRD-Raven.md)
- [PRD Section 5.3 - Task Management](docs/prd/PRD-Raven.md)