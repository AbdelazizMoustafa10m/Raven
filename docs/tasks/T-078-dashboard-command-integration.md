# T-078: Raven Dashboard Command and TUI Integration Testing

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Large: 16-24hrs |
| Dependencies | T-006, T-066, T-067, T-068, T-069, T-070, T-071, T-072, T-073, T-074, T-075, T-076, T-077 |
| Blocked By | T-076, T-077 |
| Blocks | None |

## Goal
Implement the `raven dashboard` Cobra command in `internal/cli/dashboard.go` that launches the TUI Command Center, wiring together all TUI sub-models (T-066 through T-077) with the workflow engine, agent adapters, and implementation loop from earlier phases. This task also includes the adapter layer that bridges backend events to TUI messages, the integration of the App model with all sub-models, and comprehensive integration testing of the complete TUI.

## Background
Per PRD Section 5.11, `raven dashboard` launches the TUI command center. Per PRD Section 5.12, the TUI is a "presentation layer only -- it calls the same workflow engine and agent adapters as the CLI." This task connects the presentation layer (Phase 6 TUI components) with the execution layer (Phases 2-5 workflow engine, agents, loop, review, pipeline).

The dashboard command:
1. Loads configuration from `raven.toml`
2. Initializes the workflow engine, agent adapters, and task system
3. Creates the TUI `App` with all sub-models
4. Starts the `tea.Program` with event bridging goroutines
5. Handles graceful shutdown (finish current agent call, checkpoint state)

Event bridging: backend components emit events on Go channels. Bridge goroutines read from these channels and convert events to TUI message types, sending them to the `tea.Program` via `p.Send()`.

## Technical Specifications
### Implementation Approach
Create `internal/cli/dashboard.go` containing the `raven dashboard` Cobra command. Create `internal/tui/bridge.go` containing the event bridge that converts workflow/agent/loop events to TUI messages. Update `internal/tui/app.go` to integrate all sub-models created in T-070 through T-077 into the App model, replacing the placeholder stubs.

### Key Components
- **Dashboard command**: Cobra command with flags for initial workflow, phase, etc.
- **EventBridge**: Goroutines that bridge backend event channels to `p.Send()` TUI messages
- **App integration**: Wire all sub-models into the App's Update and View methods
- **Graceful shutdown**: Context cancellation, agent process termination, state checkpointing

### API/Interface Contracts
```go
// internal/cli/dashboard.go

import (
    "github.com/spf13/cobra"
)

// NewDashboardCmd creates the "raven dashboard" command.
func NewDashboardCmd() *cobra.Command

// dashboardRun is the RunE function for the dashboard command.
func dashboardRun(cmd *cobra.Command, args []string) error

// --- internal/tui/bridge.go ---

import (
    tea "github.com/charmbracelet/bubbletea"
)

// EventBridge converts backend events to TUI messages via p.Send().
type EventBridge struct {
    program *tea.Program
    ctx     context.Context
    cancel  context.CancelFunc
}

// NewEventBridge creates a bridge that sends messages to the given program.
func NewEventBridge(program *tea.Program, ctx context.Context) *EventBridge

// BridgeWorkflowEvents reads from a workflow event channel and sends
// WorkflowEventMsg to the TUI.
func (eb *EventBridge) BridgeWorkflowEvents(ch <-chan workflow.Event)

// BridgeAgentOutput reads from an agent output channel and sends
// AgentOutputMsg to the TUI.
func (eb *EventBridge) BridgeAgentOutput(ch <-chan agent.OutputEvent)

// BridgeLoopEvents reads from a loop event channel and sends
// LoopEventMsg to the TUI.
func (eb *EventBridge) BridgeLoopEvents(ch <-chan loop.Event)

// BridgeRateLimits reads from a rate-limit channel and sends
// RateLimitMsg to the TUI.
func (eb *EventBridge) BridgeRateLimits(ch <-chan agent.RateLimitEvent)

// Stop cancels all bridge goroutines.
func (eb *EventBridge) Stop()

// --- internal/tui/app.go (updated) ---

// App is the fully integrated top-level model.
type App struct {
    config      AppConfig
    width       int
    height      int
    focus       FocusPanel
    ready       bool
    quitting    bool

    // Sub-models (fully integrated)
    layout      Layout
    sidebar     SidebarModel
    agentPanel  AgentPanelModel
    eventLog    EventLogModel
    statusBar   StatusBarModel
    helpOverlay HelpOverlay
    wizard      WizardModel
    keyMap      KeyMap
    theme       Theme
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| charmbracelet/bubbletea | v1.2+ | tea.Program, tea.Model |
| spf13/cobra | v1.10+ | Command registration |
| internal/tui/* | - | All TUI sub-models |
| internal/config | - | Configuration loading |
| internal/workflow | - | Workflow engine (if available) |
| internal/agent | - | Agent adapters (if available) |
| internal/loop | - | Implementation loop (if available) |
| internal/task | - | Task system (if available) |

## Acceptance Criteria
- [ ] `raven dashboard` command is registered as a subcommand of the root command
- [ ] Command loads `raven.toml` configuration and initializes all sub-systems
- [ ] TUI launches in alt-screen mode with all panels rendered correctly
- [ ] Event bridge goroutines start for each active backend channel
- [ ] WorkflowEventMsg messages flow from workflow engine to sidebar and event log
- [ ] AgentOutputMsg messages flow from agent adapters to agent panel
- [ ] LoopEventMsg messages flow from implementation loop to status bar and event log
- [ ] RateLimitMsg messages flow from rate-limit coordinator to sidebar
- [ ] TaskProgressMsg messages flow from task system to sidebar progress bars
- [ ] All keyboard navigation works: Tab focus cycling, scrolling, pause, skip, quit
- [ ] Help overlay shows and hides correctly
- [ ] Pipeline wizard launches and returns config
- [ ] Graceful shutdown: `q`/`Ctrl+C` cancels context, waits for current agent, checkpoints state
- [ ] `raven dashboard --dry-run` shows what would be launched without starting the TUI
- [ ] Integration tests cover the full message flow from event source to rendered view
- [ ] `go build ./...` and `go vet ./...` pass cleanly

## Testing Requirements
### Unit Tests
- `NewDashboardCmd` returns a valid Cobra command with correct name and flags
- `EventBridge` converts `workflow.Event` to `WorkflowEventMsg` correctly
- `EventBridge` converts `agent.OutputEvent` to `AgentOutputMsg` correctly
- `EventBridge` converts `loop.Event` to `LoopEventMsg` correctly
- `EventBridge` converts `agent.RateLimitEvent` to `RateLimitMsg` correctly
- `EventBridge.Stop()` causes all bridge goroutines to exit
- App.Update with each message type routes to correct sub-model
- App.View composes all sub-model views via the layout manager
- App.Update with `PauseRequestMsg` toggles pause state
- App.Update with `SkipRequestMsg` sends skip signal to workflow
- App.Update with `WizardCompleteMsg` starts pipeline with wizard config

### Integration Tests
- Full lifecycle test: create App, send WindowSizeMsg, send agent output, verify rendered screen
- Multi-agent test: send output for 3 agents, verify tab switching works
- Rate-limit lifecycle: send RateLimitMsg, tick countdown to zero, verify sidebar transitions
- Wizard flow: activate wizard, simulate form completion, verify WizardCompleteMsg
- Graceful shutdown: send quit key, verify context is cancelled
- Event bridge: create bridge with mock channels, send events, verify TUI receives messages

### Edge Cases to Handle
- Backend channels closed before TUI exits (bridge goroutines should exit cleanly, not panic)
- TUI exits before backend finishes (context cancellation should propagate)
- No workflow engine available (TUI shows "idle" state, wizard can be used to start one)
- Configuration errors (display error in TUI, do not crash)
- Rate-limit event with no active agent panel (buffer, display when agent appears)
- Multiple workflows running simultaneously (sidebar shows all, agent panel tabs)

## Implementation Notes
### Recommended Approach
1. **Dashboard command** (`internal/cli/dashboard.go`):
   ```go
   func NewDashboardCmd() *cobra.Command {
       cmd := &cobra.Command{
           Use:   "dashboard",
           Short: "Launch the TUI command center",
           RunE:  dashboardRun,
       }
       cmd.Flags().Bool("dry-run", false, "Show planned setup without launching TUI")
       return cmd
   }
   ```
2. **Event bridge** (`internal/tui/bridge.go`):
   - Each `BridgeXxxEvents` method runs in a goroutine
   - Uses `select` with `ctx.Done()` for clean shutdown:
     ```go
     func (eb *EventBridge) BridgeAgentOutput(ch <-chan agent.OutputEvent) {
         go func() {
             for {
                 select {
                 case <-eb.ctx.Done():
                     return
                 case evt, ok := <-ch:
                     if !ok { return }
                     eb.program.Send(AgentOutputMsg{
                         Agent:     evt.Agent,
                         Line:      evt.Line,
                         Stream:    evt.Stream,
                         Timestamp: evt.Timestamp,
                     })
                 }
             }
         }()
     }
     ```
3. **App integration** (update `internal/tui/app.go`):
   - Replace placeholder comments with actual sub-model fields
   - In `Init()`: return batch of sub-model Init commands
   - In `Update()`:
     - Handle `tea.WindowSizeMsg`: update layout, propagate dimensions to all sub-models
     - Handle `tea.KeyMsg`: dispatch through keyMap (T-076 logic)
     - Handle TUI messages: route to appropriate sub-model
     - Collect and batch returned Cmds
   - In `View()`:
     - If help overlay visible: render overlay on top
     - If wizard active: render wizard in main area
     - Otherwise: render layout with all sub-model views
4. **Graceful shutdown**:
   - On quit key: set `quitting = true`, cancel context
   - Return `tea.Quit` after brief delay to allow agent cleanup
   - Checkpoint workflow state before exit
5. **Register command**: add to root command in `internal/cli/root.go`

### Potential Pitfalls
- The `tea.Program` must be created before the event bridge (bridge needs `p.Send()`). But the `tea.Program` needs the initial `App` model. Solution: create the App first, then the Program, then the bridge, then start the Program with `p.Run()`. The bridge goroutines start sending messages immediately but that is fine -- Bubble Tea buffers messages until the program starts.
- Bridge goroutines must NOT call `p.Send()` after the program has exited. Use the context cancellation to stop bridges before program shutdown. The typical pattern: cancel context -> bridges exit -> call `p.Quit()` -> `p.Run()` returns.
- If backend packages from earlier phases are not yet implemented when this task is started, use interface types and mock implementations. The bridge should depend on channel types, not concrete implementations.
- The App model must delegate messages to ALL sub-models that care about them, not just the focused one. For example, `AgentOutputMsg` should always go to the agent panel regardless of focus. Only key events are focus-dependent.
- Use `tea.Batch()` to combine multiple `tea.Cmd` values returned from sub-model Updates.

### Security Considerations
- The dashboard command should not require elevated permissions
- Graceful shutdown must ensure no orphan agent processes are left running
- Workflow state checkpoint must be written atomically (same as headless mode)

## References
- [Bubble Tea Program.Send() for External Messages](https://github.com/charmbracelet/bubbletea/blob/main/examples/send-msg/main.go)
- [Cobra Command Documentation](https://pkg.go.dev/github.com/spf13/cobra)
- [PRD Section 5.11 - raven dashboard Command](docs/prd/PRD-Raven.md)
- [PRD Section 5.12 - TUI Command Center](docs/prd/PRD-Raven.md)
- [PRD Section 6.3 - Concurrency Model](docs/prd/PRD-Raven.md)