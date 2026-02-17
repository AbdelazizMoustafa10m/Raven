# T-069: Split-Pane Layout Manager

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-066, T-068 |
| Blocked By | T-068 |
| Blocks | T-070, T-071, T-072, T-073, T-074, T-075 |

## Goal
Implement the split-pane layout manager in `internal/tui/layout.go` that divides the terminal into the PRD-specified regions: title bar (top), sidebar (left), main area (right, split into agent panel and event log), and status bar (bottom). The layout dynamically resizes all panels when the terminal is resized, enforces a minimum terminal size of 80x24, and provides dimension calculations that sub-models use to render themselves at the correct size.

## Background
Per PRD Section 5.12, the TUI uses a split-pane layout:
```
+---------------------------------------------------+
| Title Bar (1 line)                                 |
+---------------+-----------------------------------+
| Sidebar       | Agent Panel (upper right)          |
| (fixed width) |                                    |
|               |-----------------------------------|
|               | Event Log (lower right)            |
+---------------+-----------------------------------+
| Status Bar (1 line)                                |
+---------------------------------------------------+
```

The sidebar has a fixed width (configurable, default 22 characters). The main area fills the remaining horizontal space and is split vertically between the agent panel (upper ~65%) and event log (lower ~35%). The title bar and status bar are each 1 line tall. All dimensions are recalculated on every `tea.WindowSizeMsg`.

Bubble Tea handles terminal resize by sending `tea.WindowSizeMsg` with the new dimensions. The layout manager computes panel dimensions from these and provides them to sub-models.

## Technical Specifications
### Implementation Approach
Create `internal/tui/layout.go` containing a `Layout` struct that computes and stores panel dimensions. The layout is recalculated whenever terminal size changes. Each panel's dimensions are exposed as `PanelDimensions` structs containing width, height, and position information. The `Render` method composes sub-model views into the final screen layout using `lipgloss.JoinHorizontal` and `lipgloss.JoinVertical`.

### Key Components
- **Layout**: Manages terminal dimensions and computes panel sizes
- **PanelDimensions**: Width and height for a single panel
- **Render**: Composes sub-model view strings into the final screen layout
- **MinTerminalSize**: Constants for minimum supported dimensions (80x24)

### API/Interface Contracts
```go
// internal/tui/layout.go

import "github.com/charmbracelet/lipgloss"

const (
    MinTerminalWidth  = 80
    MinTerminalHeight = 24
    DefaultSidebarWidth = 22
    TitleBarHeight      = 1
    StatusBarHeight     = 1
    BorderWidth         = 1  // For border characters between panels
)

// PanelDimensions holds the calculated dimensions for a single panel.
type PanelDimensions struct {
    Width  int
    Height int
}

// Layout manages the split-pane layout for the TUI.
type Layout struct {
    termWidth     int
    termHeight    int
    sidebarWidth  int
    agentSplit    float64 // Fraction of main area height for agent panel (0.0-1.0)

    // Calculated panel dimensions
    TitleBar      PanelDimensions
    Sidebar       PanelDimensions
    AgentPanel    PanelDimensions
    EventLog      PanelDimensions
    StatusBar     PanelDimensions
}

// NewLayout creates a layout with default sidebar width and agent panel split.
func NewLayout() Layout

// Resize recalculates all panel dimensions for the given terminal size.
// Returns false if the terminal is smaller than the minimum size.
func (l *Layout) Resize(width, height int) bool

// IsTooSmall returns true if the current terminal size is below minimum.
func (l Layout) IsTooSmall() bool

// TerminalSize returns the current terminal width and height.
func (l Layout) TerminalSize() (int, int)

// Render composes the final screen from individual panel view strings.
// Each parameter is the rendered output of that panel's View() method.
// The layout handles joining, borders, and padding.
func (l Layout) Render(
    theme Theme,
    titleBar string,
    sidebar string,
    agentPanel string,
    eventLog string,
    statusBar string,
) string

// RenderTooSmall returns a centered message for terminals below minimum size.
func (l Layout) RenderTooSmall(theme Theme) string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/lipgloss | v1.0+ | Layout composition (JoinHorizontal, JoinVertical, Place) |
| charmbracelet/bubbletea | v1.2+ | tea.WindowSizeMsg for resize events |

## Acceptance Criteria
- [ ] `Resize(120, 40)` correctly calculates dimensions for all 5 panels
- [ ] Sidebar width is fixed at `DefaultSidebarWidth` (22)
- [ ] Agent panel gets ~65% of the main area height; event log gets ~35%
- [ ] Title bar and status bar are each 1 line tall, full terminal width
- [ ] `Resize(79, 24)` returns false (below minimum width)
- [ ] `Resize(80, 23)` returns false (below minimum height)
- [ ] `Resize(80, 24)` returns true (exact minimum)
- [ ] `Render` composes panel views into a correctly sized string
- [ ] No panel dimension is ever zero or negative (clamped to minimum of 1)
- [ ] `RenderTooSmall` returns a centered warning message
- [ ] Layout adapts correctly from 80x24 up to 250x80
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- `NewLayout` creates layout with default sidebar width (22) and agent split (0.65)
- `Resize(120, 40)`: sidebar=22, main area width=120-22-1(border)=97, title bar height=1, status bar height=1, content area height=40-2=38, agent panel height=~25, event log height=~13
- `Resize(80, 24)`: minimum size, verify all panels have positive dimensions
- `Resize(79, 24)` returns false
- `Resize(200, 80)`: large terminal, verify dimensions scale correctly
- Panel width sum equals terminal width (sidebar + border + main = termWidth)
- Panel height sum equals terminal height (title + content + status = termHeight)
- `Render` with known panel strings produces output matching expected composition
- `RenderTooSmall` contains "too small" message

### Integration Tests
- Render a complete screen with placeholder panel content at multiple terminal sizes
- Verify no trailing whitespace or extra newlines in rendered output

### Edge Cases to Handle
- Terminal resize to exactly minimum size (80x24) -- all panels must fit
- Very wide terminal (300+ columns) -- main area should expand, sidebar stays fixed
- Very tall terminal (100+ rows) -- agent panel and event log split proportionally
- Odd terminal heights (agent/event split may not divide evenly -- handle remainder)
- Terminal shrinks below minimum while TUI is running -- show "too small" message

## Implementation Notes
### Recommended Approach
1. Define constants for minimum sizes and fixed dimensions
2. Implement `Resize`:
   - If width < MinTerminalWidth or height < MinTerminalHeight, mark too small, return false
   - Calculate content height: `termHeight - TitleBarHeight - StatusBarHeight`
   - Calculate main area width: `termWidth - sidebarWidth - BorderWidth`
   - Calculate agent panel height: `int(float64(contentHeight) * agentSplit)`
   - Calculate event log height: `contentHeight - agentPanelHeight`
   - Clamp all dimensions to minimum of 1
3. Implement `Render` using lipgloss composition:
   - Build title bar at full width
   - Build middle section: `lipgloss.JoinHorizontal(lipgloss.Top, sidebar, border, mainArea)`
   - Build main area: `lipgloss.JoinVertical(lipgloss.Left, agentPanel, divider, eventLog)`
   - Build full screen: `lipgloss.JoinVertical(lipgloss.Left, titleBar, middle, statusBar)`
4. Use lipgloss `.Width()` and `.Height()` to enforce exact panel sizes

### Potential Pitfalls
- `lipgloss.JoinHorizontal` and `lipgloss.JoinVertical` do not pad content to fill space. Each panel view must be exactly the right dimensions, or use `lipgloss.Place` to center/pad content within a fixed area.
- Off-by-one errors in dimension calculations are the most common bug. Write dimension-sum assertions in tests.
- The vertical divider between sidebar and main area can be a styled `lipgloss.Border` or a simple `|` character column. Using a single character column avoids complex border math.
- Remember to account for the horizontal divider between agent panel and event log (1 line) in height calculations.
- Use `lipgloss.NewStyle().MaxWidth(w).MaxHeight(h)` to truncate panel content that exceeds allocated space, preventing layout overflow.

### Security Considerations
- No security considerations for layout code

## References
- [Lipgloss JoinHorizontal/JoinVertical](https://pkg.go.dev/github.com/charmbracelet/lipgloss#JoinHorizontal)
- [Lipgloss Place function](https://pkg.go.dev/github.com/charmbracelet/lipgloss#Place)
- [Bubble Tea table-resize example](https://github.com/charmbracelet/bubbletea/blob/main/examples/table-resize/main.go)
- [Bubble Tea responsive layout discussion](https://github.com/charmbracelet/bubbletea/discussions/303)
- [PRD Section 5.12 - Layout Specification](docs/prd/PRD-Raven.md)