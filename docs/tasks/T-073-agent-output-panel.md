# T-073: Agent Output Panel with Viewport Scrolling and Tabbed Multi-Agent View

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Large: 16-24hrs |
| Dependencies | T-066, T-067, T-068, T-069 |
| Blocked By | T-069 |
| Blocks | T-078 |

## Goal
Implement the agent output panel (upper right area of the main panel) that displays live-scrolling agent output with a tabbed interface for switching between multiple concurrent agents. Each agent has its own output buffer (capped at 1000 lines), a viewport for scrolling through output history, and a header showing the agent name, current task, and status. When multiple agents run in parallel (e.g., during multi-agent review), tabs at the top allow switching between agent views.

## Background
Per PRD Section 5.12, the right panel's upper section shows:
```
[Active Agent Panel]
Agent: claude (implement)
Task: T-007
> Working on auth middleware...
> Modified internal/auth/...
> Committing changes...
```

The agent panel receives `AgentOutputMsg` (T-067) for each line of agent output and `AgentStatusMsg` for lifecycle changes. During parallel review (PRD Section 5.5), multiple agents may be running simultaneously, requiring tabs to switch views. The `bubbles/viewport` component provides scrollable content with keyboard navigation (j/k, up/down, PgUp/PgDn, Home/End).

Agent output buffers are capped at the last 1000 lines per agent to prevent memory growth (PRD Section 5.12 technical considerations).

## Technical Specifications
### Implementation Approach
Create `internal/tui/agent_panel.go` containing an `AgentPanelModel` sub-model. Each agent is tracked in a map of `AgentView` structs, each containing a `viewport.Model` from `charmbracelet/bubbles` and a ring buffer of output lines. Tabs are rendered at the top when multiple agents are present. Tab/Shift-Tab switches the active agent view. The viewport auto-scrolls to the bottom on new output (unless the user has manually scrolled up).

### Key Components
- **AgentPanelModel**: Top-level sub-model managing multiple agent views
- **AgentView**: Per-agent state with output buffer, viewport, status, and current task
- **OutputBuffer**: Ring buffer capped at 1000 lines per agent
- **Tab bar**: Rendered when 2+ agents are active, showing agent names with active indicator

### API/Interface Contracts
```go
// internal/tui/agent_panel.go

import (
    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

const MaxOutputLines = 1000

// OutputBuffer is a ring buffer for agent output lines.
type OutputBuffer struct {
    lines []string
    start int // ring buffer start index
    count int // number of valid lines
    cap   int // maximum capacity
}

// NewOutputBuffer creates a buffer with the given capacity.
func NewOutputBuffer(capacity int) OutputBuffer

// Append adds a line to the buffer, evicting the oldest if at capacity.
func (b *OutputBuffer) Append(line string)

// Lines returns all lines in order (oldest to newest).
func (b OutputBuffer) Lines() []string

// Len returns the number of lines in the buffer.
func (b OutputBuffer) Len() int

// AgentView holds the display state for a single agent.
type AgentView struct {
    name       string
    status     AgentStatus
    task       string         // Current task ID
    detail     string         // Current activity description
    viewport   viewport.Model
    buffer     OutputBuffer
    autoScroll bool           // Auto-scroll to bottom on new lines
}

// AgentPanelModel manages the tabbed agent output display.
type AgentPanelModel struct {
    theme      Theme
    width      int
    height     int
    focused    bool

    agents     map[string]*AgentView // keyed by agent name
    agentOrder []string              // stable tab order
    activeTab  int                   // index into agentOrder
}

// NewAgentPanelModel creates an agent panel with the given theme.
func NewAgentPanelModel(theme Theme) AgentPanelModel

// SetDimensions updates the panel size and resizes all viewports.
func (ap *AgentPanelModel) SetDimensions(width, height int)

// SetFocused sets whether the agent panel has keyboard focus.
func (ap *AgentPanelModel) SetFocused(focused bool)

// Update processes agent-related messages.
func (ap AgentPanelModel) Update(msg tea.Msg) (AgentPanelModel, tea.Cmd)

// View renders the agent panel as a string of the configured dimensions.
func (ap AgentPanelModel) View() string

// ActiveAgent returns the name of the currently viewed agent, or "".
func (ap AgentPanelModel) ActiveAgent() string

// tabBarView renders the tab bar when multiple agents are present.
func (ap AgentPanelModel) tabBarView() string

// agentHeaderView renders the header for the active agent.
func (ap AgentPanelModel) agentHeaderView() string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/bubbletea | v1.2+ | tea.Model, tea.Msg, tea.Cmd |
| charmbracelet/bubbles/viewport | latest | Scrollable content area |
| charmbracelet/lipgloss | v1.0+ | Styling tabs, headers, output |
| internal/tui (T-067) | - | AgentOutputMsg, AgentStatusMsg |
| internal/tui (T-068) | - | Theme styles |

## Acceptance Criteria
- [ ] Agent output lines appear in real-time as `AgentOutputMsg` messages arrive
- [ ] Each agent has a separate output buffer capped at 1000 lines
- [ ] Viewport auto-scrolls to bottom on new output when user is at bottom
- [ ] Viewport stops auto-scrolling when user scrolls up (manual scroll mode)
- [ ] Viewport resumes auto-scrolling when user scrolls back to bottom
- [ ] j/k, up/down, PgUp/PgDn, Home/End scroll the viewport when panel is focused
- [ ] Tab/Shift-Tab switches between agent tabs when multiple agents exist
- [ ] Tab bar renders at top of panel with active tab visually distinguished
- [ ] Agent header shows: agent name, role (implement/review), current task ID
- [ ] Agent status changes (running, completed, rate_limited) update the header
- [ ] When only one agent is present, no tab bar is shown
- [ ] New agents are added to the tab order as they appear
- [ ] Buffer eviction works correctly (oldest lines removed when at 1000)
- [ ] Panel renders cleanly at minimum size (30x10) and maximum size (200x60)
- [ ] Unit tests achieve 85% coverage

## Testing Requirements
### Unit Tests
- `NewOutputBuffer(5)`: append 3 lines, `Lines()` returns 3 lines in order
- `OutputBuffer`: append 7 lines to capacity-5 buffer, `Lines()` returns last 5
- `OutputBuffer.Len()` returns correct count before and after eviction
- `NewAgentPanelModel` creates panel with no agents
- `Update` with `AgentOutputMsg{Agent: "claude", Line: "hello"}` adds line to claude's buffer
- `Update` with `AgentOutputMsg` for unknown agent creates new AgentView
- `Update` with `AgentStatusMsg` updates agent status in header
- `Update` with `tea.KeyMsg(tea.KeyTab)` when focused switches active tab
- `Update` with `tea.KeyMsg(tea.KeyShiftTab)` when focused switches tab backwards
- Tab switching wraps around (last -> first, first -> last)
- `ActiveAgent` returns correct agent name
- `View()` with no agents shows placeholder message
- `View()` with one agent shows header + output, no tab bar
- `View()` with two agents shows tab bar + header + output
- `SetDimensions` resizes viewport correctly

### Integration Tests
- Simulate rapid agent output (100 lines/second) and verify buffer does not exceed 1000
- Send interleaved output from multiple agents and verify correct agent receives correct lines

### Edge Cases to Handle
- Agent output line longer than panel width (viewport handles horizontal truncation)
- Agent output containing ANSI escape codes (pass through -- viewport handles them)
- Agent output containing tab characters (render as spaces)
- Receiving output for agent that was previously completed (re-add to view)
- Empty agent output line (still append, renders as blank line)
- All agents completed (show last active agent's output)
- Very rapid output (100+ lines per second) -- buffer append must be fast (O(1))

## Implementation Notes
### Recommended Approach
1. Implement `OutputBuffer` as a ring buffer:
   - Use a fixed-size slice with `start` and `count` fields
   - `Append`: if `count < cap`, add at `(start + count) % cap` and increment count; else add at `start % cap` and increment start
   - `Lines()`: return slice in order from oldest to newest
2. Implement `AgentView`:
   - Create `viewport.New(width, height-headerLines)` for each agent
   - Store `autoScroll = true` initially
   - On new output: append to buffer, rebuild viewport content, if autoScroll then `viewport.GotoBottom()`
3. Implement `AgentPanelModel`:
   - `agents` map and `agentOrder` slice for stable ordering
   - On `AgentOutputMsg`: find or create AgentView, append line, update viewport
   - On `AgentStatusMsg`: update agent's status and task fields
   - On `tea.KeyMsg(Tab/ShiftTab)`: cycle `activeTab` through `agentOrder`
   - On viewport navigation keys (j/k/arrows/pgup/pgdn): forward to active viewport
4. Implement `View()`:
   - If no agents: render centered "Waiting for agents..." message
   - If 2+ agents: render tab bar at top
   - Render agent header (name, task, status)
   - Render active agent's viewport
   - Pad/truncate to exact panel dimensions
5. Auto-scroll detection:
   - After user scrolls, check `viewport.AtBottom()` -- if true, set `autoScroll = true`
   - If user scrolls up, set `autoScroll = false`
   - On new output with `autoScroll = true`, call `viewport.GotoBottom()`

### Potential Pitfalls
- The `viewport.Model` from bubbles requires content to be set as a single string (`viewport.SetContent(strings.Join(lines, "\n"))`). Rebuilding this string on every line of output can be expensive. Optimize by only rebuilding when the viewport is visible (active tab).
- When resizing the panel (`SetDimensions`), each agent's viewport must also be resized via `viewport.Width = newWidth; viewport.Height = newHeight`. Failure to do this causes rendering glitches.
- The viewport's internal scroll position may need adjustment after resize. Call `viewport.GotoBottom()` if auto-scroll is active after resize.
- Tab key conflicts: `Tab` is also used for focus switching between panels (T-076). The agent panel should only process Tab when it is focused AND there are 2+ agents. Otherwise, pass the key through to the parent.
- Do NOT rebuild viewport content from the full 1000-line buffer on every single `AgentOutputMsg`. Instead, append to the content string and only rebuild from scratch on resize or tab switch.

### Security Considerations
- Agent output may contain sensitive information (API keys, tokens, file contents). The TUI displays it as-is since it is a local terminal application. Do not log viewport content to files from the TUI layer.

## References
- [Bubbles Viewport Component](https://pkg.go.dev/github.com/charmbracelet/bubbles/viewport)
- [Bubble Tea Pager Example](https://github.com/charmbracelet/bubbletea/blob/main/examples/pager/main.go)
- [PRD Section 5.12 - Agent Output Panel](docs/prd/PRD-Raven.md)
- [PRD Section 5.12 - Output Buffer Cap (1000 lines)](docs/prd/PRD-Raven.md)