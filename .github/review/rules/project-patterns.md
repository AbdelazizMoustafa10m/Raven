# Raven Project Patterns

Reference patterns for reviewing Raven changes.

## Go and Error Handling

- Wrap errors with context: `fmt.Errorf("loading config: %w", err)`.
- Avoid panic except unrecoverable startup failures.
- Prefer explicit dependency injection; avoid global mutable state.
- Use `io.Reader`/`io.Writer` interfaces for testable units.
- Return errors, never panic from library code.

## Constructor Pattern

```go
func New(opts Options) (*Thing, error) {
    if opts.Required == "" {
        return nil, fmt.Errorf("required field missing")
    }
    return &Thing{...}, nil
}
```

## Interface-Driven Design

Define interfaces where consumers need them. Keep them small (1-3 methods).

```go
// In the consumer package, not the provider
type TaskSelector interface {
    SelectNext(ctx context.Context, phase int) (*Task, error)
}
```

## Cobra CLI Contracts

- Command flags must have deterministic defaults and clear help text.
- CLI output contracts keep machine-readable output stable.
- Diagnostics use `charmbracelet/log`, never `fmt.Printf` for status.
- Progress/status to stderr, structured output to stdout.
- Exit codes: 0=success, 1=error, 2=partial, 3=cancelled.

## Subprocess Execution

```go
cmd := exec.CommandContext(ctx, "claude", args...)
cmd.Dir = workDir
cmd.Env = append(os.Environ(), extraEnv...)
// Always pass args as separate strings - never shell concatenation
// Always use CommandContext for cancellation support
```

## Bounded Parallelism (errgroup)

```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(concurrency)
for _, item := range items {
    g.Go(func() error {
        // ctx is cancelled if any goroutine returns error
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        return processItem(ctx, item)
    })
}
return g.Wait()
```

## Channel Lifecycle

```go
// Producer owns the channel - creates, writes, closes
ch := make(chan Event, bufferSize)
go func() {
    defer close(ch) // Always close when done producing
    for item := range items {
        select {
        case ch <- item:
        case <-ctx.Done():
            return
        }
    }
}()

// Consumer reads until closed
for event := range ch {
    handle(event)
}
```

## Goroutine Safety

- Every goroutine must have a clear shutdown path (context cancellation or channel close).
- Use `sync.WaitGroup` or `errgroup` to wait for goroutines - never fire-and-forget.
- Protect shared state with mutex or serialize through channels - never concurrent map access.
- Agent output buffers are capped (last N lines) to prevent memory growth.

## TUI Bubble Tea Patterns

```go
// All state mutations go through Update() - Elm architecture
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case AgentOutputMsg:
        m.agentBuffer = append(m.agentBuffer, msg.Line)
        // Cap buffer size
        if len(m.agentBuffer) > maxLines {
            m.agentBuffer = m.agentBuffer[len(m.agentBuffer)-maxLines:]
        }
    }
    return m, nil
}

// View() is a pure function - no side effects, no mutation
func (m Model) View() string {
    return lipgloss.JoinVertical(lipgloss.Left, m.sidebar(), m.mainPanel())
}
```

## TUI Event Streaming from Goroutines

```go
// Agent goroutines send messages via p.Send()
go func() {
    scanner := bufio.NewScanner(stdout)
    for scanner.Scan() {
        p.Send(AgentOutputMsg{Agent: "claude", Line: scanner.Text()})
    }
}()
```

## Workflow State Machine

```go
// Transitions are explicit - no implicit fallthrough
type Transition struct {
    OnSuccess string
    OnFailure string
    OnBlocked string
}

// Checkpoint after every transition
func (e *Engine) transition(state *WorkflowState, event Event) error {
    next, ok := e.transitions[state.CurrentStep][event]
    if !ok {
        return fmt.Errorf("no transition from %s on %s", state.CurrentStep, event)
    }
    state.CurrentStep = next
    return e.checkpoint(state) // Persist before proceeding
}
```

## Determinism and Reproducibility

- Sort map/slice outputs before rendering or serializing.
- Use `json.Encoder` with `SetIndent` for stable checkpoint JSON.
- Task state file updates should be atomic (write to temp, rename).
- Keep ordering stable for same input across runs.

## Config Resolution

```go
// Resolution order: CLI flags > env vars > raven.toml > defaults
// CLI flags always win
func Resolve(flags *Flags, env Environment, file *TomlConfig, defaults *Config) *Config {
    // Merge in reverse priority, higher wins
}
```

## Security and Safety

- Secret redaction paths must fail closed, not fail open.
- Regex and parsing logic must avoid unbounded behaviors (ReDoS).
- Script/tool invocations must avoid shell injection vectors.
- Avoid logging secrets, tokens, auth headers, or raw credentials.
- Validate all file paths constructed from user input or config.

## Testing Standards

- Prefer table-driven tests for behavior matrices.
- Add golden tests for stable output formats (review reports, status output).
- Use `t.TempDir()` and fixture data in `testdata/`.
- Cover negative/error paths, not only happy paths.
- Use mock agents for integration tests - no real API calls in CI.
