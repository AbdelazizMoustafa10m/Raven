# T-070: Sidebar -- Workflow List with Status Indicators

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-8hrs |
| Dependencies | T-066, T-067, T-068, T-069 |
| Blocked By | T-069 |
| Blocks | T-078 |

## Goal
Implement the workflow list section of the sidebar panel in `internal/tui/sidebar.go`, displaying active and available workflows with real-time status indicators. This is the topmost section of the sidebar showing which workflows are running, pending, or completed, with Unicode status symbols styled per the theme.

## Background
Per PRD Section 5.12, the left sidebar panel displays:
```
Workflows
  ● pipeline    (active/running)
  ○ review      (idle/available)
```

Workflows are tracked by the workflow engine (Phase 4 tasks). The sidebar receives `WorkflowEventMsg` messages (T-067) to update workflow statuses. Each workflow shows its name and a status indicator. The currently selected workflow is highlighted, and selecting a workflow switches the agent panel to show that workflow's active agent output.

The sidebar is a composite sub-model containing three sections: workflow list (this task), task progress (T-071), and rate-limit status (T-072). This task implements the overall sidebar `tea.Model` container and the workflow list section.

## Technical Specifications
### Implementation Approach
Create `internal/tui/sidebar.go` containing a `SidebarModel` that implements a Bubble Tea sub-model pattern (Init, Update, View). The sidebar manages its own state: a list of tracked workflows with their statuses, the currently selected workflow index, and scroll position if the list overflows the sidebar height. It receives and processes `WorkflowEventMsg` and `FocusChangedMsg` messages.

### Key Components
- **SidebarModel**: Composite sub-model containing workflow list, task progress, and rate-limit sections
- **WorkflowEntry**: State for a single tracked workflow (name, status, start time)
- **WorkflowStatus**: Enum for workflow display states (running, completed, failed, idle, paused)

### API/Interface Contracts
```go
// internal/tui/sidebar.go

// WorkflowStatus represents the display state of a workflow.
type WorkflowStatus int

const (
    WorkflowIdle WorkflowStatus = iota
    WorkflowRunning
    WorkflowPaused
    WorkflowCompleted
    WorkflowFailed
)

// WorkflowEntry tracks the state of a single workflow in the sidebar.
type WorkflowEntry struct {
    Name      string
    Status    WorkflowStatus
    StartedAt time.Time
    Detail    string // e.g., "Phase 2" or "T-007"
}

// SidebarModel is the Bubble Tea sub-model for the left sidebar panel.
type SidebarModel struct {
    theme          Theme
    width          int
    height         int
    focused        bool

    // Workflow list section
    workflows      []WorkflowEntry
    selectedIdx    int

    // Sub-sections (populated by T-071, T-072)
    // taskProgress   TaskProgressModel  // T-071
    // rateLimits     RateLimitModel     // T-072
}

// NewSidebarModel creates a sidebar with the given theme.
func NewSidebarModel(theme Theme) SidebarModel

// SetDimensions updates the sidebar panel size.
func (s *SidebarModel) SetDimensions(width, height int)

// SetFocused sets whether the sidebar has keyboard focus.
func (s *SidebarModel) SetFocused(focused bool)

// Update processes messages relevant to the sidebar.
func (s SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd)

// View renders the sidebar panel as a string of the configured dimensions.
func (s SidebarModel) View() string

// SelectedWorkflow returns the name of the currently selected workflow, or "".
func (s SidebarModel) SelectedWorkflow() string

// workflowListView renders just the workflow list section.
func (s SidebarModel) workflowListView() string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/bubbletea | v1.2+ | tea.Msg, tea.Cmd |
| charmbracelet/lipgloss | v1.0+ | Styling for workflow entries and status indicators |
| internal/tui (T-067) | - | WorkflowEventMsg, FocusChangedMsg |
| internal/tui (T-068) | - | Theme, StatusIndicator |

## Acceptance Criteria
- [ ] SidebarModel displays a "Workflows" section header
- [ ] Each workflow entry shows a status indicator and name (e.g., "● pipeline")
- [ ] Status indicators use correct Unicode symbols: ● (running), ◌ (paused), ✓ (completed), ✗ (failed), ○ (idle)
- [ ] Status indicators are colored per the Theme (green=running, yellow=paused, red=failed, etc.)
- [ ] Selected workflow is highlighted with a distinct background/foreground
- [ ] Arrow keys (j/k or up/down) move selection when sidebar is focused
- [ ] `WorkflowEventMsg` updates the correct workflow entry's status
- [ ] New workflows are automatically added to the list when first seen
- [ ] Workflow list scrolls if it exceeds available height
- [ ] `SetDimensions` correctly constrains the rendered view to the specified width/height
- [ ] Text longer than sidebar width is truncated (not wrapped)
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- `NewSidebarModel` creates model with empty workflow list
- `Update` with `WorkflowEventMsg` for new workflow adds it to the list
- `Update` with `WorkflowEventMsg` for existing workflow updates its status
- `Update` with `tea.KeyMsg("j")` when focused moves selection down
- `Update` with `tea.KeyMsg("k")` when focused moves selection up
- Selection wraps or clamps at list boundaries
- `View()` output width does not exceed configured width
- `View()` output height does not exceed configured height
- `SelectedWorkflow()` returns correct name
- `SelectedWorkflow()` returns empty string when list is empty
- Workflow status indicators render correct Unicode symbols

### Integration Tests
- Send sequence of WorkflowEventMsg messages and verify sidebar view updates correctly

### Edge Cases to Handle
- Empty workflow list (show placeholder text like "No workflows")
- Very long workflow name (truncate with ellipsis)
- Many workflows exceeding sidebar height (scroll with visual indicator)
- Receiving WorkflowEventMsg before sidebar is dimensioned (buffer events)
- Duplicate workflow events (idempotent status update)

## Implementation Notes
### Recommended Approach
1. Define `WorkflowStatus` enum with `String()` method
2. Define `WorkflowEntry` struct
3. Implement `SidebarModel` with workflows slice and selectedIdx
4. In `Update`, handle:
   - `WorkflowEventMsg`: find workflow by name, update status or append new entry
   - `tea.KeyMsg`: if focused, handle j/k/up/down for selection
   - `FocusChangedMsg`: update focused flag
5. In `workflowListView`:
   - Render "Workflows" header with sidebar title style
   - For each workflow, render: `  {indicator} {name}`
   - Apply highlight style to selected workflow
   - Truncate names to `width - 4` (2 indent + indicator + space)
6. In `View`, compose all sidebar sections vertically:
   - Workflow list (this task)
   - Empty line separator
   - Task progress placeholder (T-071)
   - Empty line separator
   - Rate limit placeholder (T-072)
   - Pad to full height
7. Apply sidebar border/container style from theme

### Potential Pitfalls
- The sidebar sub-model pattern: `Update` returns `(SidebarModel, tea.Cmd)` not `(tea.Model, tea.Cmd)`. The parent `App.Update` must call this and replace its sidebar field.
- Do not process key events when `!focused` -- pass them through to avoid capturing keys meant for other panels.
- `lipgloss.Width(str)` counts visual width (accounting for ANSI codes). Use this for truncation, not `len(str)`.
- When scrolling the workflow list, calculate visible window based on `height - headerLines - separators`.

### Security Considerations
- Workflow names come from configuration and are not user-controlled at runtime; no sanitization needed

## References
- [Bubble Tea Sub-model Pattern](https://github.com/charmbracelet/bubbletea/tree/main/tutorials/basics)
- [Lipgloss Width function](https://pkg.go.dev/github.com/charmbracelet/lipgloss#Width)
- [PRD Section 5.12 - Sidebar Specification](docs/prd/PRD-Raven.md)