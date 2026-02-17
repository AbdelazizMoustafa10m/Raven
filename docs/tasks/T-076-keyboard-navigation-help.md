# T-076: Keyboard Navigation and Help Overlay

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-066, T-067, T-068, T-069, T-070, T-073, T-074 |
| Blocked By | T-073, T-074 |
| Blocks | T-078 |

## Goal
Implement the keyboard navigation system and help overlay for the TUI Command Center in `internal/tui/keybindings.go`. This includes panel focus cycling (Tab/Shift-Tab between sidebar, agent panel, event log), global action keys (p for pause, s for skip, q for quit, l for log toggle), scrolling delegation to the focused panel, and a `?` key-activated help overlay showing all available keybindings.

## Background
Per PRD Section 5.12, the TUI supports these controls:
- `Tab` / `Shift+Tab` -- Switch between agent panels (and between focus panels)
- `p` -- Pause/resume workflow
- `s` -- Skip current task
- `q` / `Ctrl+C` -- Graceful shutdown
- `?` -- Help overlay
- `l` -- Toggle log panel
- Arrow keys / `j`/`k` -- Scroll agent output / event log
- `PgUp`/`PgDn`/`Home`/`End` -- Page scrolling in focused viewport

The keyboard system must distinguish between global keys (always active), panel-specific keys (only active when that panel has focus), and overlay keys (only active when help overlay is shown). Focus cycling follows a predictable order: sidebar -> agent panel -> event log -> sidebar.

## Technical Specifications
### Implementation Approach
Create `internal/tui/keybindings.go` containing a `KeyMap` struct that defines all keybindings using `charmbracelet/bubbles/key` binding definitions, a `HelpOverlay` model for the help screen, and a `HandleKeyMsg` function that the top-level `App.Update` calls to dispatch key events based on focus state and overlay visibility.

### Key Components
- **KeyMap**: Defines all keybindings with descriptions for the help overlay
- **HelpOverlay**: Full-screen overlay showing keybinding reference, dismissible with `?` or `Esc`
- **HandleKeyMsg**: Central key dispatch logic for the App model
- **Focus cycling**: Tab/Shift-Tab logic for moving focus between panels

### API/Interface Contracts
```go
// internal/tui/keybindings.go

import (
    "github.com/charmbracelet/bubbles/key"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

// KeyMap defines all keybindings for the TUI.
type KeyMap struct {
    // Global keys (always active)
    Quit        key.Binding
    Help        key.Binding
    Pause       key.Binding
    Skip        key.Binding
    ToggleLog   key.Binding
    FocusNext   key.Binding
    FocusPrev   key.Binding

    // Scrolling keys (active in focused viewport panel)
    Up          key.Binding
    Down        key.Binding
    PageUp      key.Binding
    PageDown    key.Binding
    Home        key.Binding
    End         key.Binding

    // Agent panel keys (active when agent panel focused)
    NextAgent   key.Binding
    PrevAgent   key.Binding
}

// DefaultKeyMap returns the default keybinding configuration.
func DefaultKeyMap() KeyMap

// HelpOverlay displays the keybinding reference.
type HelpOverlay struct {
    theme   Theme
    keyMap  KeyMap
    visible bool
    width   int
    height  int
}

// NewHelpOverlay creates a help overlay with the given theme and keymap.
func NewHelpOverlay(theme Theme, keyMap KeyMap) HelpOverlay

// SetDimensions updates the overlay size.
func (h *HelpOverlay) SetDimensions(width, height int)

// Toggle shows or hides the overlay.
func (h *HelpOverlay) Toggle()

// IsVisible returns whether the overlay is currently shown.
func (h HelpOverlay) IsVisible() bool

// Update processes key events when the overlay is visible.
func (h HelpOverlay) Update(msg tea.Msg) (HelpOverlay, tea.Cmd)

// View renders the help overlay as a full-screen string.
func (h HelpOverlay) View() string

// --- Focus Management ---

// NextFocus returns the next panel in the focus cycle.
func NextFocus(current FocusPanel) FocusPanel

// PrevFocus returns the previous panel in the focus cycle.
func PrevFocus(current FocusPanel) FocusPanel

// --- Key Dispatch ---

// PauseRequestMsg is sent when the user presses 'p' to pause/resume.
type PauseRequestMsg struct{}

// SkipRequestMsg is sent when the user presses 's' to skip current task.
type SkipRequestMsg struct{}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/bubbletea | v1.2+ | tea.Msg, tea.KeyMsg |
| charmbracelet/bubbles/key | latest | key.Binding definitions |
| charmbracelet/lipgloss | v1.0+ | Help overlay styling |
| internal/tui (T-067) | - | FocusChangedMsg |
| internal/tui (T-068) | - | Theme (HelpKey, HelpDesc styles) |

## Acceptance Criteria
- [ ] `DefaultKeyMap()` defines all keybindings listed in PRD Section 5.12
- [ ] `Tab` cycles focus: sidebar -> agent panel -> event log -> sidebar
- [ ] `Shift+Tab` cycles focus in reverse
- [ ] Focus changes are communicated via `FocusChangedMsg`
- [ ] `p` sends a `PauseRequestMsg` (handled by App to pause/resume workflow)
- [ ] `s` sends a `SkipRequestMsg` (handled by App to skip current task)
- [ ] `q` and `Ctrl+C` and `Ctrl+Q` trigger graceful quit
- [ ] `l` toggles the event log panel visibility
- [ ] `?` toggles the help overlay
- [ ] Help overlay shows all keybindings organized by category
- [ ] Help overlay is dismissible with `?` or `Esc`
- [ ] When help overlay is visible, all other keys are suppressed (except `?` and `Esc`)
- [ ] Scrolling keys (j/k/arrows/pgup/pgdn/home/end) are forwarded only to the focused panel
- [ ] `p` and `s` keys do NOT activate when help overlay is visible
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- `DefaultKeyMap()` returns KeyMap with all bindings non-empty
- `NextFocus(FocusSidebar)` returns `FocusAgentPanel`
- `NextFocus(FocusAgentPanel)` returns `FocusEventLog`
- `NextFocus(FocusEventLog)` returns `FocusSidebar`
- `PrevFocus(FocusSidebar)` returns `FocusEventLog`
- `PrevFocus(FocusEventLog)` returns `FocusAgentPanel`
- `HelpOverlay.Toggle()` flips visibility
- `HelpOverlay.View()` contains all keybinding descriptions
- `HelpOverlay.View()` width/height matches configured dimensions
- `HelpOverlay.Update` with `Esc` key hides the overlay
- `HelpOverlay.Update` with `?` key hides the overlay
- Key bindings match: `key.Matches(msg, keyMap.Quit)` returns true for 'q'
- Key bindings match: `key.Matches(msg, keyMap.Quit)` returns true for ctrl+c

### Integration Tests
- Simulate Tab key presses and verify focus cycles through all panels
- Simulate `?` key press, verify overlay appears, press `Esc`, verify overlay hides

### Edge Cases to Handle
- Rapid key presses (Tab-Tab-Tab) should cycle focus correctly without skipping
- Help overlay on very small terminal (80x24) -- content may need scrolling
- Key events while no workflow is running (pause/skip are no-ops, not errors)
- Multiple quit signals (Ctrl+C pressed rapidly) -- should not panic or double-quit

## Implementation Notes
### Recommended Approach
1. Define `KeyMap` struct using `key.Binding` from `charmbracelet/bubbles/key`:
   ```go
   Quit: key.NewBinding(
       key.WithKeys("q", "ctrl+c", "ctrl+q"),
       key.WithHelp("q/ctrl+c", "quit"),
   )
   ```
2. Implement `DefaultKeyMap()` populating all bindings
3. Implement `NextFocus`/`PrevFocus` with a simple modular arithmetic cycle
4. Implement `HelpOverlay`:
   - `View()`: render a centered box with keybinding categories
   - Categories: "Navigation", "Actions", "Scrolling"
   - Each entry: `theme.HelpKey.Render(keyName) + "  " + theme.HelpDesc.Render(description)`
   - Use `lipgloss.Place` to center the overlay on screen
5. Integrate into `App.Update`:
   ```go
   case tea.KeyMsg:
       if app.helpOverlay.IsVisible() {
           app.helpOverlay, cmd = app.helpOverlay.Update(msg)
           return app, cmd
       }
       switch {
       case key.Matches(msg, app.keyMap.Help):
           app.helpOverlay.Toggle()
       case key.Matches(msg, app.keyMap.Quit):
           // graceful shutdown
       case key.Matches(msg, app.keyMap.FocusNext):
           app.focus = NextFocus(app.focus)
           // send FocusChangedMsg to sub-models
       case key.Matches(msg, app.keyMap.Pause):
           return app, func() tea.Msg { return PauseRequestMsg{} }
       // ... delegate scroll keys to focused panel
       }
   ```
6. Define `PauseRequestMsg` and `SkipRequestMsg` as message types for the workflow integration layer (T-078) to handle

### Potential Pitfalls
- `key.Matches` uses `key.Binding` definitions. Make sure key names match Bubble Tea's key name format: `"ctrl+c"` not `"Ctrl+C"`, `"tab"` not `"Tab"`.
- The `Tab` key in Bubble Tea v1.x is `tea.KeyTab`. In `key.Binding`, use `key.WithKeys("tab")`.
- `Shift+Tab` is `tea.KeyShiftTab` or `key.WithKeys("shift+tab")`.
- The help overlay should be rendered ON TOP of the existing TUI (not replacing it). In `App.View()`, check `helpOverlay.IsVisible()` and if true, render the overlay using `lipgloss.Place` over the full terminal.
- Be careful with the `l` key -- it should NOT toggle the log when the user is typing in a text input (e.g., wizard). Gate on `app.mode != modeWizard` or similar.

### Security Considerations
- Key bindings are hardcoded; no user-configurable keybindings in v2.0 (avoids injection via config)

## References
- [Bubbles Key Binding Package](https://pkg.go.dev/github.com/charmbracelet/bubbles/key)
- [Bubble Tea Key Message Handling](https://github.com/charmbracelet/bubbletea/blob/main/tutorials/basics/README.md)
- [PRD Section 5.12 - TUI Controls](docs/prd/PRD-Raven.md)