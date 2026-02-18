# T-089: Stream-JSON Integration -- Wire StreamDecoder into Agent Adapters and Loop Runner

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-022, T-027, T-088 |
| Blocked By | T-022, T-027, T-088 |
| Blocks | T-067, T-073 |

## Goal
Wire the `StreamDecoder` (T-088) into `ClaudeAgent.Run()` so that when callers provide a `StreamEvents` channel via `RunOpts`, the adapter decodes JSONL from stdout in real-time and forwards typed `StreamEvent` values. Additionally, enhance the implementation loop runner (T-027) to optionally consume stream events and emit fine-grained `LoopEvent` types for tool-call-level observability. This bridges the parsing layer (T-088) with the execution layer (T-022, T-027), making stream data available to all downstream consumers (TUI, headless automation, review pipeline).

## Background
T-088 delivered the `StreamDecoder`, `StreamEvent` types, and helper methods in `internal/agent/stream.go`. However, T-022 (`ClaudeAgent`) and T-027 (loop runner) were implemented on a separate branch **before** T-088, so they never incorporated streaming. The result is:

1. **`ClaudeAgent.Run()`** reads stdout into a `bytes.Buffer` as a monolithic blob (`claude.go:118-121`). It ignores `RunOpts.StreamEvents` entirely.
2. **`RunOpts.StreamEvents`** channel field exists in `types.go` but is never written to by any adapter.
3. **Loop runner** (`runner.go:632`) calls `r.agent.Run(ctx, opts)` and only inspects the final `RunResult.Stdout` for signal detection. It has no access to individual tool calls or reasoning text during execution.

This task completes the data flow: `claude CLI stdout` → `StreamDecoder` → `StreamEvents channel` → loop runner / TUI / automation consumers.

### Why Now (Phase 8, Not Phase 6)
The TUI tasks (T-067, T-073) will **consume** stream events but should not need to worry about **producing** them. By wiring the producer side now in Phase 8, the TUI tasks can simply read from the `StreamEvents` channel with confidence that it carries data. Deferring this to Phase 6 would force TUI developers to also modify the agent adapter — mixing concerns.

## Technical Specifications

### Part 1: ClaudeAgent Streaming Integration

#### Current Flow (claude.go:102-161)
```
cmd.StdoutPipe() → goroutine reads into bytes.Buffer → cmd.Wait() → return RunResult{Stdout: buf.String()}
```

#### Target Flow (when StreamEvents is non-nil)
```
cmd.StdoutPipe() → io.TeeReader(pipe, &stdoutBuf) → StreamDecoder.Next() loop → send to StreamEvents channel
                                                    ↓ (on EOF)
                                                    return RunResult{Stdout: stdoutBuf.String()}
```

When `opts.StreamEvents` is nil, behavior is unchanged (backward compatible).

#### Implementation in ClaudeAgent.Run()
```go
// internal/agent/claude.go -- modifications to Run()

func (c *ClaudeAgent) Run(ctx context.Context, opts RunOpts) (*RunResult, error) {
    start := time.Now()
    cmd := c.buildCommand(ctx, opts)
    // ... existing logger call ...

    stdoutPipe, err := cmd.StdoutPipe()
    if err != nil {
        return nil, fmt.Errorf("creating stdout pipe: %w", err)
    }
    stderrPipe, err := cmd.StderrPipe()
    if err != nil {
        return nil, fmt.Errorf("creating stderr pipe: %w", err)
    }

    var (
        stdoutBuf bytes.Buffer
        stderrBuf bytes.Buffer
        wg        sync.WaitGroup
    )

    wg.Add(1)
    go func() {
        defer wg.Done()
        _, _ = stderrBuf.ReadFrom(stderrPipe)
    }()

    // Stdout handling: stream-decode or buffer depending on opts.
    wg.Add(1)
    if opts.StreamEvents != nil && opts.OutputFormat == OutputFormatStreamJSON {
        go func() {
            defer wg.Done()
            // TeeReader: every byte read by the decoder is also written to stdoutBuf.
            tee := io.TeeReader(stdoutPipe, &stdoutBuf)
            decoder := NewStreamDecoder(tee)
            for {
                event, err := decoder.Next()
                if err == io.EOF {
                    return
                }
                if err != nil {
                    // Log and skip malformed lines.
                    if c.logger != nil {
                        c.logger.Debug("stream decode error", "err", err)
                    }
                    continue
                }
                // Non-blocking send: if consumer is slow, skip event rather than
                // blocking the subprocess pipe (which would stall the agent).
                select {
                case opts.StreamEvents <- *event:
                default:
                    if c.logger != nil {
                        c.logger.Debug("stream event dropped (channel full)", "type", event.Type)
                    }
                }
            }
        }()
    } else {
        go func() {
            defer wg.Done()
            _, _ = stdoutBuf.ReadFrom(stdoutPipe)
        }()
    }

    if err := cmd.Start(); err != nil {
        wg.Wait()
        return nil, fmt.Errorf("starting claude: %w", err)
    }

    wg.Wait()
    // ... existing cmd.Wait(), exit code, rate-limit parsing, return ...
}
```

#### Key Design Decisions
1. **`io.TeeReader`** -- Every byte the `StreamDecoder` reads from stdout is also captured in `stdoutBuf`, so `RunResult.Stdout` still contains the full output for rate-limit parsing and signal detection. No data loss.
2. **Non-blocking channel send** -- Uses `select` with `default` to prevent a slow consumer from stalling the subprocess pipe. Events are dropped rather than blocking the agent process. The channel should be buffered (recommended: 256) by the caller.
3. **Guard on `OutputFormat == OutputFormatStreamJSON`** -- Only activates streaming when the caller explicitly requests stream-json format. A `StreamEvents` channel with `OutputFormat: "json"` (non-streaming) falls back to the buffer path since the stdout is a single JSON blob, not JSONL.
4. **Backward compatible** -- When `StreamEvents` is nil, the existing `ReadFrom` buffer path executes unchanged. Zero behavioral change for all existing callers.

### Part 2: Codex Adapter Consideration
The `CodexAgent` currently does not support `--output-format stream-json`. Codex uses a different output format. The `StreamEvents` field on `RunOpts` should be silently ignored by `CodexAgent.Run()` — no changes needed there. Add a doc comment to clarify this.

### Part 3: Loop Runner Stream Consumption

#### New LoopEvent Types
```go
// internal/loop/runner.go -- new event types

const (
    // ... existing event types ...
    EventToolStarted   LoopEventType = "tool_started"    // Agent invoked a tool (e.g., Read, Edit, Bash)
    EventToolCompleted LoopEventType = "tool_completed"  // Tool result received
    EventAgentThinking LoopEventType = "agent_thinking"  // Agent emitted reasoning text
    EventSessionStats  LoopEventType = "session_stats"   // End-of-session cost/token stats
)
```

#### New LoopEvent Fields
```go
type LoopEvent struct {
    // ... existing fields ...
    ToolName   string  // For EventToolStarted/EventToolCompleted
    CostUSD    float64 // For EventSessionStats
    TokensIn   int     // For EventSessionStats
    TokensOut  int     // For EventSessionStats
}
```

#### Stream Consumer in Loop Runner
Add a `consumeStreamEvents` method to `Runner` that reads from a `StreamEvents` channel and translates `StreamEvent` values into `LoopEvent` emissions:

```go
// consumeStreamEvents translates stream events into loop events.
// Runs in a goroutine, exits when the events channel is closed.
func (r *Runner) consumeStreamEvents(events <-chan agent.StreamEvent, iteration int, taskID string) {
    for event := range events {
        switch event.Type {
        case agent.StreamEventAssistant:
            for _, block := range event.ToolUseBlocks() {
                r.emitEvent(LoopEvent{
                    Type:      EventToolStarted,
                    Iteration: iteration,
                    TaskID:    taskID,
                    ToolName:  block.Name,
                    Message:   fmt.Sprintf("Tool: %s", block.Name),
                    Timestamp: time.Now(),
                })
            }
            if text := event.TextContent(); text != "" {
                r.emitEvent(LoopEvent{
                    Type:      EventAgentThinking,
                    Iteration: iteration,
                    TaskID:    taskID,
                    Message:   truncate(text, 200),
                    Timestamp: time.Now(),
                })
            }
        case agent.StreamEventUser:
            for _, block := range event.ToolResultBlocks() {
                r.emitEvent(LoopEvent{
                    Type:      EventToolCompleted,
                    Iteration: iteration,
                    TaskID:    taskID,
                    ToolName:  block.Name,
                    Timestamp: time.Now(),
                })
            }
        case agent.StreamEventResult:
            r.emitEvent(LoopEvent{
                Type:      EventSessionStats,
                Iteration: iteration,
                TaskID:    taskID,
                CostUSD:   event.CostUSD,
                TokensIn:  tokenCount(event),
                TokensOut:  tokenCount(event),
                Message:   fmt.Sprintf("Cost: $%.4f, Turns: %d", event.CostUSD, event.NumTurns),
                Timestamp: time.Now(),
            })
        }
    }
}
```

#### Integration in `invokeAgent`
Modify the `invokeAgent` method to optionally create a `StreamEvents` channel, pass it via `RunOpts`, and launch `consumeStreamEvents` in a goroutine:

```go
func (r *Runner) invokeAgent(ctx context.Context, runCfg RunConfig, prompt string) (*agent.RunResult, error) {
    opts := agent.RunOpts{
        Prompt:       prompt,
        OutputFormat: agent.OutputFormatStreamJSON,
    }

    // Set up streaming if events channel is available.
    var streamCh chan agent.StreamEvent
    if r.events != nil { // r.events is the loop's event channel -- if a consumer exists, enable streaming
        streamCh = make(chan agent.StreamEvent, 256)
        opts.StreamEvents = streamCh
        go r.consumeStreamEvents(streamCh, runCfg.currentIteration, runCfg.currentTaskID)
    }

    result, err := r.agent.Run(ctx, opts)
    if err != nil {
        return nil, fmt.Errorf("invoking agent %s: %w", runCfg.AgentName, err)
    }
    return result, nil
}
```

**Note:** The `StreamEvents` channel is closed implicitly when the agent's stdout pipe hits EOF and the goroutine in `ClaudeAgent.Run()` exits. However, per the T-088 contract, the **caller owns the channel**. Since the `consumeStreamEvents` goroutine reads until the channel is closed, and the channel is closed when `invokeAgent` returns (the goroutine in ClaudeAgent exits), this is safe. If a more explicit lifecycle is needed, the caller can close the channel after `Run()` returns.

### Part 4: Review Pipeline Stream Integration (Optional Enhancement)
The review orchestrator (T-035, not yet built) will also call `agent.Run()`. When it does, it can pass `StreamEvents` channels to get per-agent tool-call visibility during reviews. This requires no additional changes — the orchestrator just needs to set `RunOpts.OutputFormat` and `RunOpts.StreamEvents`. Documenting this here for T-035's implementer.

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| io (stdlib) | - | TeeReader for dual-path stdout |
| internal/agent/stream.go (T-088) | - | StreamDecoder, StreamEvent types |
| internal/agent/claude.go (T-022) | - | ClaudeAgent.Run() modification |
| internal/loop/runner.go (T-027) | - | Loop runner stream consumption |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria

### ClaudeAgent Integration
- [ ] When `RunOpts.StreamEvents` is non-nil and `OutputFormat` is `stream-json`, `ClaudeAgent.Run()` decodes stdout JSONL and sends `StreamEvent` values to the channel
- [ ] `RunResult.Stdout` still contains the full raw stdout (via `io.TeeReader`) for rate-limit parsing and signal detection
- [ ] When `RunOpts.StreamEvents` is nil, behavior is identical to the pre-integration implementation
- [ ] When `RunOpts.OutputFormat` is not `stream-json`, `StreamEvents` is ignored (buffer path used)
- [ ] Non-blocking channel send: if the channel is full, events are dropped (not blocked)
- [ ] Malformed JSONL lines are logged at debug level and skipped
- [ ] No goroutine leaks: the streaming goroutine exits when the stdout pipe closes

### Loop Runner Integration
- [ ] New `LoopEventType` constants: `EventToolStarted`, `EventToolCompleted`, `EventAgentThinking`, `EventSessionStats`
- [ ] New `LoopEvent` fields: `ToolName`, `CostUSD`, `TokensIn`, `TokensOut`
- [ ] `consumeStreamEvents` translates assistant tool_use blocks into `EventToolStarted`
- [ ] `consumeStreamEvents` translates user tool_result blocks into `EventToolCompleted`
- [ ] `consumeStreamEvents` translates assistant text blocks into `EventAgentThinking`
- [ ] `consumeStreamEvents` translates result events into `EventSessionStats`
- [ ] When no events channel exists on the runner, streaming is not activated (OutputFormat stays as-is)
- [ ] `invokeAgent` creates a buffered channel (256) and launches `consumeStreamEvents` in a goroutine

### Backward Compatibility
- [ ] All existing tests pass without modification
- [ ] `MockAgent` continues to work (it ignores `StreamEvents`)
- [ ] `CodexAgent` and `GeminiAgent` silently ignore `StreamEvents`

## Testing Requirements

### Unit Tests -- ClaudeAgent Streaming (Table-Driven)
- `Run()` with nil StreamEvents: identical behavior to baseline (buffer path)
- `Run()` with StreamEvents + OutputFormat json (not stream-json): buffer path used, no events sent
- `Run()` with StreamEvents + OutputFormat stream-json: events decoded and sent to channel
- Stream with 3 JSONL events: all 3 received on channel
- Stream with malformed line in middle: 2 valid events received, malformed skipped
- Stream with very large tool result (>64KB line): event decoded successfully
- Full channel (capacity 1): events dropped, subprocess not blocked
- RunResult.Stdout contains full raw output even when streaming
- Rate-limit detection still works on RunResult after streaming
- Context cancellation: goroutine exits cleanly

### Unit Tests -- Loop Runner Stream Consumption (Table-Driven)
- Assistant event with tool_use block: EventToolStarted emitted with correct ToolName
- Assistant event with text block: EventAgentThinking emitted with truncated text
- User event with tool_result block: EventToolCompleted emitted
- Result event: EventSessionStats emitted with CostUSD and token counts
- Mixed event stream (system, assistant, user, result): correct LoopEvents in order
- Empty stream (channel closed immediately): no events emitted, no panic
- Runner with nil events channel: streaming not activated, OutputFormat unchanged

### Integration Tests
- Full round-trip: mock subprocess producing JSONL → ClaudeAgent.Run() with StreamEvents → verify events match input
- Loop runner with mock agent producing stream events → verify LoopEvents emitted

### Edge Cases
- Agent exits with non-zero code mid-stream: partial events delivered, RunResult captures exit code
- Agent produces zero stdout (empty stream): StreamEvents channel receives nothing, RunResult.Stdout is empty
- Concurrent access: multiple goroutines reading LoopEvents while stream consumer writes (channel serializes)
- StreamEvents channel closed by caller before Run() completes: non-blocking send prevents panic (send to closed channel is caught by select/default — actually this would panic; document that callers must NOT close the channel before Run returns)

## Implementation Notes

### Recommended Approach
1. **Start with ClaudeAgent.Run() modification** — this is the producer side
   a. Add the `io.TeeReader` + `StreamDecoder` goroutine path
   b. Write tests using a mock subprocess (exec a small Go program that outputs JSONL)
   c. Verify `RunResult.Stdout` still contains full output
2. **Add new LoopEvent types and fields** — pure additive, no breaking changes
3. **Implement `consumeStreamEvents`** — the consumer side in the loop runner
4. **Modify `invokeAgent`** to wire the channel and goroutine
5. **Verify all existing tests pass** — the key backward-compatibility check

### Testing the Subprocess
For testing `ClaudeAgent.Run()` with streaming, use the standard Go test pattern of exec-ing the test binary itself:
```go
func TestClaudeAgent_RunStreaming(t *testing.T) {
    if os.Getenv("TEST_CLAUDE_SUBPROCESS") == "1" {
        // This process acts as a fake claude CLI.
        fmt.Println(`{"type":"system","subtype":"init","session_id":"test-123","model":"claude-4-sonnet"}`)
        fmt.Println(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"thinking..."}]}}`)
        fmt.Println(`{"type":"result","cost_usd":0.01,"num_turns":1}`)
        return
    }
    // ... set up ClaudeAgent with Command pointing to os.Args[0] and env TEST_CLAUDE_SUBPROCESS=1 ...
}
```

### Potential Pitfalls
- **Panic on send to closed channel**: The `StreamEvents` channel must NOT be closed by the caller while `Run()` is still executing. Document this clearly. The non-blocking select/default does NOT protect against closed channels — `select { case ch <- v: default: }` on a closed channel panics.
- **TeeReader EOF synchronization**: `io.TeeReader` returns EOF when the underlying reader (stdout pipe) returns EOF. The `StreamDecoder` then returns `io.EOF` from `Next()`, and the goroutine exits. This naturally synchronizes with `cmd.Wait()` because Go's `exec.Cmd` closes the pipe write-end when the process exits.
- **Buffer size for StreamEvents channel**: Recommend 256. Too small risks dropping events during bursts of tool calls; too large wastes memory. Callers can tune this.
- **`OutputFormat` interaction**: When the loop runner enables streaming, it changes `OutputFormat` from empty/json to `stream-json`. This changes what Claude CLI writes to stdout (JSONL instead of a single JSON blob). Ensure signal detection (`DetectSignals`) still works — it scans for `PHASE_COMPLETE` etc. in `RunResult.Stdout`, which now contains JSONL lines rather than plain text. The signals would appear inside assistant text blocks. **This may require updating `DetectSignals` to scan JSONL content or extracting text from the buffered stdout.** Test this carefully.

### Security Considerations
- Stream events may contain sensitive source code in tool inputs/results. The `consumeStreamEvents` method truncates reasoning text in `EventAgentThinking` messages to 200 chars, but `ToolName` is always safe to log.
- Do not log full `StreamEvent` at info level — use debug only.

## References
- [T-088 Task Spec](T-088-headless-observability.md) -- StreamDecoder and StreamEvent types
- [T-022 Task Spec](T-022-claude-agent-adapter.md) -- ClaudeAgent.Run() implementation
- [T-027 Task Spec](T-027-implementation-loop-runner.md) -- Loop runner and LoopEvent system
- [T-067 Task Spec](T-067-tui-message-types.md) -- Downstream TUI message consumer
- [T-073 Task Spec](T-073-agent-output-panel.md) -- Downstream TUI agent panel consumer
- [Go io.TeeReader](https://pkg.go.dev/io#TeeReader)
- [Go os/exec test pattern](https://npf.io/2015/06/testing-exec-command/)
- [PRD Section 5.2 - Agent Adapter System](docs/prd/PRD-Raven.md)
- [PRD Section 10 Q3 - CLI vs API decision](docs/prd/PRD-Raven.md)
