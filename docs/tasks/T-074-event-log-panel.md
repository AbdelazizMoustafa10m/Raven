# T-074: Event Log Panel for Workflow Milestones

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-066, T-067, T-068, T-069 |
| Blocked By | T-069 |
| Blocks | T-078 |

## Goal
Implement the event log panel (lower right area of the main panel) that displays a chronological stream of workflow milestones, agent lifecycle events, and system notifications. The event log provides a concise, timestamped history of significant events across all workflows and agents, serving as a central audit trail visible alongside the agent output panel.

## Background
Per PRD Section 5.12, the lower right panel shows:
```
[Agent Log / Event Stream]
14:23:01 Agent started T-007
14:23:45 Rate limit detected
14:23:45 Waiting 120s...
14:25:45 Retrying...
14:26:02 T-007 completed
```

Unlike the agent output panel which shows raw agent stdout, the event log shows high-level, human-readable milestone events: task starts/completions, rate-limit events, workflow step transitions, errors, and system notifications. Events come from `WorkflowEventMsg`, `LoopEventMsg`, `AgentStatusMsg`, `RateLimitMsg`, and `ErrorMsg` (all defined in T-067).

The event log uses a `viewport.Model` for scrolling and is toggleable via the `l` key (PRD Section 5.12 controls).

## Technical Specifications
### Implementation Approach
Create `internal/tui/event_log.go` containing an `EventLogModel` sub-model. It maintains a list of `EventEntry` structs, each with a timestamp, category, and formatted message. The viewport displays these entries with timestamps styled in the theme's muted color and messages styled by category (info, warning, error, success). The log auto-scrolls to the newest entry unless the user has scrolled up. The log is toggleable (visible/hidden) via the `l` key.

### Key Components
- **EventLogModel**: Sub-model managing the event log display
- **EventEntry**: Single log entry with timestamp, category, and message
- **EventCategory**: Classification for styling (info, warning, error, success, debug)
- **Event formatting**: Converts TUI messages into human-readable log entries

### API/Interface Contracts
```go
// internal/tui/event_log.go

import (
    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
)

const MaxEventLogEntries = 500

// EventCategory classifies events for styling purposes.
type EventCategory int

const (
    EventInfo EventCategory = iota
    EventSuccess
    EventWarning
    EventError
    EventDebug
)

// EventEntry is a single entry in the event log.
type EventEntry struct {
    Timestamp time.Time
    Category  EventCategory
    Message   string
}

// EventLogModel manages the event log viewport and entries.
type EventLogModel struct {
    theme      Theme
    width      int
    height     int
    focused    bool
    visible    bool   // toggled via 'l' key

    entries    []EventEntry
    viewport   viewport.Model
    autoScroll bool
}

// NewEventLogModel creates an event log with the given theme.
func NewEventLogModel(theme Theme) EventLogModel

// SetDimensions updates the panel size and resizes the viewport.
func (el *EventLogModel) SetDimensions(width, height int)

// SetFocused sets whether the event log has keyboard focus.
func (el *EventLogModel) SetFocused(focused bool)

// SetVisible sets whether the event log is visible.
func (el *EventLogModel) SetVisible(visible bool)

// IsVisible returns whether the event log is currently visible.
func (el EventLogModel) IsVisible() bool

// Update processes messages that produce event log entries.
func (el EventLogModel) Update(msg tea.Msg) (EventLogModel, tea.Cmd)

// View renders the event log as a string of the configured dimensions.
func (el EventLogModel) View() string

// AddEntry appends a new entry to the event log.
func (el *EventLogModel) AddEntry(category EventCategory, message string)

// formatEntry renders a single EventEntry as a styled string.
func (el EventLogModel) formatEntry(entry EventEntry) string

// classifyWorkflowEvent maps a WorkflowEventMsg to an EventCategory and message.
func classifyWorkflowEvent(msg WorkflowEventMsg) (EventCategory, string)

// classifyLoopEvent maps a LoopEventMsg to an EventCategory and message.
func classifyLoopEvent(msg LoopEventMsg) (EventCategory, string)

// classifyAgentStatus maps an AgentStatusMsg to an EventCategory and message.
func classifyAgentStatus(msg AgentStatusMsg) (EventCategory, string)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/bubbletea | v1.2+ | tea.Model, tea.Msg |
| charmbracelet/bubbles/viewport | latest | Scrollable event log |
| charmbracelet/lipgloss | v1.0+ | Timestamp and message styling |
| internal/tui (T-067) | - | All message types |
| internal/tui (T-068) | - | Theme styles |

## Acceptance Criteria
- [ ] Event log displays timestamped entries in chronological order
- [ ] Timestamps formatted as "HH:MM:SS" in the theme's muted color
- [ ] Event messages styled by category (green=success, yellow=warning, red=error, default=info)
- [ ] `WorkflowEventMsg` produces entries like "Workflow 'pipeline' step: implement -> review"
- [ ] `LoopEventMsg` produces entries like "Task T-007 selected for iteration 3"
- [ ] `AgentStatusMsg` produces entries like "Agent claude started T-007"
- [ ] `RateLimitMsg` produces entries like "Rate limit: anthropic, waiting 2:00"
- [ ] `ErrorMsg` produces entries with error styling
- [ ] Event log auto-scrolls to newest entry unless user scrolled up
- [ ] j/k, up/down scroll when focused
- [ ] `l` key toggles visibility (when not in an input field)
- [ ] Maximum 500 entries retained; oldest are evicted
- [ ] Hidden event log returns an empty string from View() (layout manager reallocates space)
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- `NewEventLogModel` creates model with empty entries, visible=true, autoScroll=true
- `AddEntry` appends entry with current timestamp
- `AddEntry` beyond MaxEventLogEntries evicts oldest entries
- `Update` with `WorkflowEventMsg` adds correctly formatted entry
- `Update` with `LoopEventMsg` (LoopTaskSelected) adds "Task T-007 selected" entry
- `Update` with `AgentStatusMsg` (AgentRunning) adds "Agent claude started" entry
- `Update` with `RateLimitMsg` adds countdown entry
- `Update` with `ErrorMsg` adds error-styled entry
- `formatEntry` includes timestamp and styled message
- `classifyWorkflowEvent` returns correct category for success/failure events
- `classifyLoopEvent` returns correct category for all LoopEventType values
- `classifyAgentStatus` maps AgentFailed to EventError
- `SetVisible(false)` makes View() return empty string
- Viewport scrolling works with j/k keys when focused

### Integration Tests
- Send 600 entries, verify only last 500 are retained and oldest are gone
- Verify auto-scroll behavior: add entries while at bottom, scroll up, add more, verify position

### Edge Cases to Handle
- Empty event log (show "No events yet" placeholder)
- Very long event messages (truncate to panel width, do not wrap)
- Rapid event burst (50+ events per second) -- buffer should handle without lag
- Event timestamps with identical times (display in insertion order)
- Event log toggled invisible then visible (viewport position preserved)
- Events arriving when log is invisible (still buffered, visible when toggled on)

## Implementation Notes
### Recommended Approach
1. Define `EventCategory` enum and `EventEntry` struct
2. Implement `EventLogModel` with entries slice, viewport, visibility flag
3. Implement classifier functions that map each message type to (EventCategory, string):
   - `WorkflowEventMsg`: "Workflow '{name}' {event}: {step}" -- category based on event field
   - `LoopEventMsg`: switch on Type for specific messages
   - `AgentStatusMsg`: "Agent {name} {status}" with task info
   - `RateLimitMsg`: "Rate limit: {provider}, waiting {countdown}"
   - `ErrorMsg`: directly use Detail field
4. In `Update`, type-switch on messages:
   - For each classifiable message, call classifier and `AddEntry`
   - For `tea.KeyMsg("l")`: toggle visibility
   - For navigation keys when focused: forward to viewport
5. In `AddEntry`:
   - Append to entries slice
   - If len > MaxEventLogEntries, trim from front
   - Rebuild viewport content (formatted entries joined by newlines)
   - If autoScroll, GotoBottom
6. In `View`:
   - If !visible, return ""
   - Render "Event Log" header
   - Render viewport below header
   - Pad to exact dimensions

### Potential Pitfalls
- Rebuilding the full viewport content string on every new entry is O(n). For 500 entries this is fast, but optimize if profiling shows issues. Consider maintaining a running content string and only appending.
- The `l` toggle key should NOT be processed when the user is in a text input field (e.g., pipeline wizard). The parent Update handler should gate this.
- When the event log is hidden, the layout manager (T-069) should reallocate its height to the agent panel. This requires coordination: `EventLogModel.IsVisible()` informs the layout's `Resize` calculation.
- Ensure the viewport height accounts for the "Event Log" header line: `viewport.Height = panelHeight - 1`.

### Security Considerations
- Event log entries may contain task IDs and agent names but no sensitive data
- Do not include raw agent output or command-line arguments in event entries

## References
- [Bubbles Viewport Component](https://pkg.go.dev/github.com/charmbracelet/bubbles/viewport)
- [PRD Section 5.12 - Event Stream Panel](docs/prd/PRD-Raven.md)
- [PRD Section 5.12 - Toggle Log with 'l' key](docs/prd/PRD-Raven.md)