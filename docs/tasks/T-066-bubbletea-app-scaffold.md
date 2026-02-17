# T-066: Bubble Tea Application Scaffold and Elm Architecture Model

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-004 |
| Blocked By | None |
| Blocks | T-067, T-068, T-069, T-070, T-071, T-072, T-073, T-074, T-075, T-076, T-077, T-078 |

## Goal
Create the foundational Bubble Tea application model in `internal/tui/app.go` that implements the Elm architecture (Model-Update-View) for the Raven TUI Command Center. This scaffold defines the top-level application state, the main `Update` dispatcher, the root `View` compositor, and the `tea.Program` lifecycle, providing the skeleton that all subsequent TUI tasks build upon.

## Background
Per PRD Section 5.12, the TUI Command Center is launched via `raven dashboard` and provides a real-time split-pane dashboard for monitoring and controlling workflows. The Bubble Tea framework enforces the Elm architecture pattern: all state lives in the Model, all mutations happen through `Update(msg) (tea.Model, tea.Cmd)`, and the entire UI is rendered as a string by `View()`. This serialization of state updates through `Update()` eliminates race conditions even with concurrent agent goroutines sending messages via `p.Send()`.

The application model aggregates sub-models for each UI panel (sidebar, agent panel, event log, status bar) and delegates messages to the appropriate sub-model based on message type and focus state. The `tea.WindowSizeMsg` is handled at the top level to coordinate responsive layout across all panels.

## Technical Specifications
### Implementation Approach
Create `internal/tui/app.go` containing the top-level `App` model that implements `tea.Model`. The App holds references to sub-models (added by subsequent tasks as interfaces/stubs), terminal dimensions, focus state, and a quit flag. It uses `tea.NewProgram` with `tea.WithAltScreen()` for full-screen rendering. The `Update` method dispatches messages to sub-models based on type, and the `View` method composes the final screen from sub-model views.

### Key Components
- **App**: Top-level `tea.Model` implementing `Init()`, `Update()`, `View()`
- **FocusState**: Enum tracking which panel has keyboard focus (sidebar, agent panel, event log)
- **AppConfig**: Configuration passed at construction time (workflow channels, initial state)
- **RunTUI**: Public function that creates and runs the `tea.Program`

### API/Interface Contracts
```go
// internal/tui/app.go

// FocusPanel identifies which panel currently has keyboard focus.
type FocusPanel int

const (
    FocusSidebar FocusPanel = iota
    FocusAgentPanel
    FocusEventLog
)

// AppConfig holds configuration for the TUI application.
type AppConfig struct {
    Version    string
    ProjectName string
    // Channels for receiving events from workflow engine / agents
    // (added as concrete types become available from T-067)
}

// App is the top-level Bubble Tea model for the Raven Command Center.
type App struct {
    config     AppConfig
    width      int
    height     int
    focus      FocusPanel
    ready      bool    // true after first WindowSizeMsg
    quitting   bool

    // Sub-model placeholders (populated by subsequent tasks)
    // sidebar    SidebarModel    // T-070, T-071, T-072
    // agentPanel AgentPanelModel // T-073
    // eventLog   EventLogModel   // T-074
    // statusBar  StatusBarModel  // T-075
}

func NewApp(cfg AppConfig) App
func (a App) Init() tea.Cmd
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (a App) View() string

// RunTUI creates a tea.Program, runs it, and returns the final model or error.
// The returned *tea.Program can be used by goroutines to send messages via p.Send().
func RunTUI(cfg AppConfig) error
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/bubbletea | v1.2+ (latest stable v1.3.x) | TUI framework, Elm architecture |
| charmbracelet/lipgloss | v1.0+ | Terminal styling (used in View) |
| internal/buildinfo | - | Version string for title bar |

## Acceptance Criteria
- [ ] `App` implements `tea.Model` interface (`Init`, `Update`, `View`)
- [ ] `NewApp` creates a properly initialized App with default focus on sidebar
- [ ] `Init()` returns a `tea.WindowSize` command (or nil) to get initial terminal dimensions
- [ ] `Update` handles `tea.WindowSizeMsg` and stores width/height, sets `ready = true`
- [ ] `Update` handles `tea.KeyMsg` for quit keys (`q`, `ctrl+c`, `ctrl+q`) and sets `quitting = true`
- [ ] `View()` returns a placeholder layout string showing title bar and panels when `ready`
- [ ] `View()` returns a "loading..." spinner or message when `!ready`
- [ ] `RunTUI` creates a `tea.Program` with `tea.WithAltScreen()` and `tea.WithMouseCellMotion()`
- [ ] Terminal minimum size check: if width < 80 or height < 24, display a "terminal too small" message
- [ ] Unit tests achieve 90% coverage for the App model
- [ ] `go build ./...` and `go vet ./...` pass cleanly

## Testing Requirements
### Unit Tests
- `NewApp` returns App with correct defaults (focus=sidebar, ready=false, quitting=false)
- `Update` with `tea.WindowSizeMsg{Width: 120, Height: 40}` sets dimensions and ready=true
- `Update` with `tea.KeyMsg{Type: tea.KeyCtrlC}` sets quitting=true and returns `tea.Quit`
- `Update` with `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}` sets quitting=true
- `View` when not ready returns loading message
- `View` when ready returns a string containing the title bar with version
- `View` when terminal too small returns warning message
- Multiple `WindowSizeMsg` updates correctly track latest dimensions

### Integration Tests
- Create App, send WindowSizeMsg, verify View output contains expected sections

### Edge Cases to Handle
- Zero-width or zero-height terminal (should show error, not panic)
- Very large terminal (200+ columns) should not panic or produce garbled output
- Rapid successive WindowSizeMsg messages should not corrupt state
- Quit during initialization (before ready) should exit cleanly

## Implementation Notes
### Recommended Approach
1. Define `FocusPanel` type and constants
2. Define `AppConfig` struct with version and project name
3. Define `App` struct with width, height, focus, ready, quitting fields
4. Implement `NewApp` setting defaults
5. Implement `Init` returning nil (WindowSizeMsg is sent automatically by bubbletea)
6. Implement `Update` with a type switch:
   - `tea.WindowSizeMsg`: store dimensions, set ready
   - `tea.KeyMsg`: handle quit keys
   - Default: no-op (sub-models added later)
7. Implement `View`:
   - If quitting: return empty string
   - If not ready: return centered "Initializing..."
   - If terminal too small: return centered warning
   - Otherwise: return placeholder with title bar and panel areas
8. Implement `RunTUI` creating `tea.NewProgram(NewApp(cfg), tea.WithAltScreen())`

### Potential Pitfalls
- Do NOT use `tea.WithMouseAllMotion()` -- it captures all mouse events including selection, which breaks copy/paste. Use `tea.WithMouseCellMotion()` for scroll wheel support only.
- The `View()` method must return a string of exactly `width * height` characters (with newlines) to avoid flickering. Use lipgloss padding/truncation to ensure this.
- Remember that `Update` returns a new `tea.Model` (value semantics). If using a struct receiver, changes must be made on the receiver and returned. Using a pointer receiver is also valid.
- Bubbletea v1.x automatically sends a `WindowSizeMsg` on startup; no need to request it in `Init()`.

### Security Considerations
- No sensitive data should be rendered in the TUI title bar
- The TUI should not log raw agent output to files without the user's knowledge (that is the logging system's responsibility)

## References
- [Bubble Tea GitHub Repository](https://github.com/charmbracelet/bubbletea)
- [Bubble Tea Basics Tutorial](https://github.com/charmbracelet/bubbletea/blob/main/tutorials/basics/README.md)
- [Bubble Tea v1.3.0 Release](https://github.com/charmbracelet/bubbletea/releases/tag/v1.3.0)
- [PRD Section 5.12 - Interactive TUI Command Center](docs/prd/PRD-Raven.md)
- [PRD Section 6.3 - Concurrency Model (TUI Event Loop)](docs/prd/PRD-Raven.md)