# T-068: Lipgloss Styles and Theme System

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-8hrs |
| Dependencies | T-066 |
| Blocked By | T-066 |
| Blocks | T-069, T-070, T-071, T-072, T-073, T-074, T-075, T-076 |

## Goal
Define the complete Lipgloss style system and theme for the Raven TUI Command Center in `internal/tui/styles.go`. This includes color palettes for light and dark terminals, reusable style constants for borders, panels, text emphasis, progress bars, status indicators, and the title bar. All subsequent TUI components reference these styles for visual consistency.

## Background
Per PRD Section 5.12, the TUI uses `charmbracelet/lipgloss` for declarative styling. Lipgloss v1.0+ provides `lipgloss.AdaptiveColor` which automatically selects between light and dark terminal color values based on the terminal's background color. This allows a single theme definition that works across both light and dark terminals without user configuration.

The style system must support the PRD's split-pane layout with clear visual separation between panels, status indicators (active/inactive workflows, OK/WAIT rate limits), progress bar theming, and a distinctive title bar.

## Technical Specifications
### Implementation Approach
Create `internal/tui/styles.go` containing a `Theme` struct that holds all Lipgloss styles used across the application. Provide a `DefaultTheme()` constructor that builds the theme using `lipgloss.AdaptiveColor` for all colors. Styles are organized by component (title bar, sidebar, agent panel, event log, status bar) and by semantic purpose (active, inactive, error, warning, success).

### Key Components
- **Theme**: Central struct holding all lipgloss.Style instances
- **Color palette**: Named `lipgloss.AdaptiveColor` values for light/dark terminals
- **Component styles**: Pre-built styles for each TUI panel and element
- **Status styles**: Styles for different states (running, completed, failed, waiting)
- **DefaultTheme()**: Constructor that builds the complete theme

### API/Interface Contracts
```go
// internal/tui/styles.go

import "github.com/charmbracelet/lipgloss"

// --- Color Palette ---

var (
    // Primary colors
    ColorPrimary     = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7B78FF"}
    ColorSecondary   = lipgloss.AdaptiveColor{Light: "#3B82F6", Dark: "#60A5FA"}
    ColorAccent      = lipgloss.AdaptiveColor{Light: "#10B981", Dark: "#34D399"}

    // Status colors
    ColorSuccess     = lipgloss.AdaptiveColor{Light: "#16A34A", Dark: "#4ADE80"}
    ColorWarning     = lipgloss.AdaptiveColor{Light: "#D97706", Dark: "#FBBF24"}
    ColorError       = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F87171"}
    ColorInfo        = lipgloss.AdaptiveColor{Light: "#2563EB", Dark: "#60A5FA"}

    // Neutral colors
    ColorMuted       = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
    ColorSubtle      = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#4B5563"}
    ColorBorder      = lipgloss.AdaptiveColor{Light: "#E5E7EB", Dark: "#374151"}
    ColorHighlight   = lipgloss.AdaptiveColor{Light: "#F3F4F6", Dark: "#1F2937"}
)

// Theme holds all styles for the TUI components.
type Theme struct {
    // Title bar
    TitleBar       lipgloss.Style
    TitleText      lipgloss.Style
    TitleVersion   lipgloss.Style
    TitleHint      lipgloss.Style

    // Sidebar
    SidebarContainer lipgloss.Style
    SidebarTitle     lipgloss.Style
    SidebarItem      lipgloss.Style
    SidebarActive    lipgloss.Style
    SidebarInactive  lipgloss.Style

    // Agent panel
    AgentContainer   lipgloss.Style
    AgentHeader      lipgloss.Style
    AgentTab         lipgloss.Style
    AgentTabActive   lipgloss.Style
    AgentOutput      lipgloss.Style

    // Event log
    EventContainer   lipgloss.Style
    EventTimestamp   lipgloss.Style
    EventMessage     lipgloss.Style

    // Status bar
    StatusBar        lipgloss.Style
    StatusKey        lipgloss.Style
    StatusValue      lipgloss.Style
    StatusSeparator  lipgloss.Style

    // Progress bars
    ProgressFilled   lipgloss.Style
    ProgressEmpty    lipgloss.Style
    ProgressLabel    lipgloss.Style
    ProgressPercent  lipgloss.Style

    // Status indicators
    StatusRunning    lipgloss.Style
    StatusCompleted  lipgloss.Style
    StatusFailed     lipgloss.Style
    StatusWaiting    lipgloss.Style
    StatusBlocked    lipgloss.Style

    // General
    Border           lipgloss.Style
    HelpKey          lipgloss.Style
    HelpDesc         lipgloss.Style
    ErrorText        lipgloss.Style

    // Dividers
    VerticalDivider  lipgloss.Style
    HorizontalDivider lipgloss.Style
}

// DefaultTheme returns the default Raven TUI theme with adaptive colors.
func DefaultTheme() Theme

// StatusIndicator returns a styled string for the given status.
// Examples: "●" (running/green), "○" (idle/muted), "!" (error/red), "◌" (waiting/yellow)
func (t Theme) StatusIndicator(status AgentStatus) string

// ProgressBar renders a text-based progress bar of the given width.
// filled is a float64 between 0.0 and 1.0.
func (t Theme) ProgressBar(filled float64, width int) string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/lipgloss | v1.0+ | Declarative terminal styling with adaptive colors |

## Acceptance Criteria
- [ ] `Theme` struct defined with styles for all TUI components listed in the PRD layout
- [ ] All colors use `lipgloss.AdaptiveColor` for light/dark terminal support
- [ ] `DefaultTheme()` returns a fully populated Theme
- [ ] `StatusIndicator` returns appropriate Unicode symbols with correct colors for each `AgentStatus`
- [ ] `ProgressBar` renders a correct text progress bar for 0%, 50%, 100%, and intermediate values
- [ ] Borders between panels are clearly visible in both light and dark terminals
- [ ] Title bar has a distinctive background color
- [ ] Status bar uses contrasting colors for key/value pairs
- [ ] Unit tests verify all Theme fields are non-zero (no missing style initialization)
- [ ] Visual spot-check: styles render correctly in at least 2 terminal emulators

## Testing Requirements
### Unit Tests
- `DefaultTheme()` returns a Theme where no style field is the zero value
- `StatusIndicator` for each `AgentStatus` returns a non-empty string
- `ProgressBar(0.0, 20)` returns a bar with no filled characters
- `ProgressBar(1.0, 20)` returns a bar fully filled
- `ProgressBar(0.5, 20)` returns approximately half filled
- `ProgressBar` with width 0 returns empty string (no panic)
- `ProgressBar` with negative width returns empty string

### Integration Tests
- Render each style to a string and verify it produces non-empty output

### Edge Cases to Handle
- Terminal with no color support (lipgloss auto-degrades, but test that styles still produce valid strings)
- Unicode support: ensure status indicators (bullet, circle) render correctly or provide ASCII fallbacks
- Very narrow progress bars (width < 3) should still render without panic

## Implementation Notes
### Recommended Approach
1. Define color palette as package-level variables using `lipgloss.AdaptiveColor`
2. Define `Theme` struct with all style fields
3. Implement `DefaultTheme()` building each style:
   - Title bar: full width, background color, bold text
   - Sidebar: fixed width border-right, padding
   - Agent panel: border, padding, flexible width
   - Status bar: full width, inverted colors
4. Implement `StatusIndicator` with a switch on `AgentStatus`
5. Implement `ProgressBar` using filled/empty block characters (e.g., `\u2588` filled, `\u2591` empty)
6. Add ASCII fallback option for terminals without Unicode support

### Potential Pitfalls
- Lipgloss styles are immutable value types. Each method call (`.Bold(true)`, `.Foreground(color)`) returns a new `lipgloss.Style`. Chain calls or assign to variables.
- Do NOT set `Width` or `Height` on styles at definition time -- those are set dynamically based on terminal size in the layout manager (T-069).
- `lipgloss.AdaptiveColor` requires the terminal to support background color detection. On terminals that don't support it, the Light color is used. Test with `TERM=dumb`.
- Progress bar characters: use `\u2588` (full block) for filled and `\u2591` (light shade) for empty. Some terminals may not render these; consider `#` and `-` as fallbacks.

### Security Considerations
- No security considerations for pure styling code

## References
- [Lipgloss GitHub Repository](https://github.com/charmbracelet/lipgloss)
- [Lipgloss AdaptiveColor Documentation](https://pkg.go.dev/github.com/charmbracelet/lipgloss#AdaptiveColor)
- [Lipgloss v1.0 Release](https://github.com/charmbracelet/lipgloss/releases)
- [PRD Section 5.12 - Lipgloss Styling](docs/prd/PRD-Raven.md)