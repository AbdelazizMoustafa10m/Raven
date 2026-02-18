# T-088: Headless Observability -- Stream-JSON Event Parsing

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-004, T-021 |
| Blocked By | T-004 |
| Blocks | T-022, T-027, T-067, T-073 |

## Goal
Implement stream-json event types and a JSONL decoder for parsing Claude Code's `--output-format stream-json` real-time event stream. This provides full headless observability into agent activity -- tool calls, reasoning text, tool results, token usage, and session metadata -- giving the TUI (or any automation consumer) the same visibility as Claude Code's interactive mode.

## Background
When running Claude Code in headless mode (`claude -p "..." --output-format stream-json --verbose`), stdout emits a JSONL stream where each line is a self-contained JSON event. The event types are:

1. **`system`** (subtype `init`) -- Session initialization: tools available, MCP servers, model name, session ID
2. **`assistant`** -- Assistant messages containing `text` (reasoning) and `tool_use` (tool calls) content blocks
3. **`user`** -- Tool results sent back to the model, containing `tool_result` content blocks
4. **`result`** -- Final session result: cost, duration, token usage, error status, turn count

This replaces the current design where agents run as black boxes returning full stdout after completion. By decoding the JSONL stream line-by-line in real-time, Raven can render:
- **Tool calls panel**: Which tools the agent is using (Read, Edit, Bash, etc.) and their inputs
- **Reasoning panel**: The agent's text reasoning as it streams
- **Output panel**: Tool results and outputs
- **Stats**: Token usage, cost, turn count
- **Session tracking**: Session ID for resume/hook correlation

The PRD (Section 10, Q3) deferred direct API access to v2.1+, but stream-json parsing over CLI subprocess stdout achieves the same observability **without** direct API calls -- it stays within the v2.0 "always shell out to CLI tools" architecture.

### Claude Code Hooks Integration
In addition to stream-json, Claude Code supports lifecycle hooks (`PreToolUse`, `PostToolUse`, `Notification`, etc.) that can write to named pipes or Unix sockets. This task establishes the stream event types that hooks can also produce, creating a unified observability contract. Hook integration itself is a follow-up task.

## Technical Specifications
### Implementation Approach
Create `internal/agent/stream.go` containing stream event types (mapping to Claude Code's JSONL schema) and a `StreamDecoder` that reads from an `io.Reader` line-by-line, decoding each JSON line into a typed `StreamEvent`. The decoder supports both synchronous iteration (`Next()`) and channel-based async consumption (`Decode()`). Malformed lines are logged and skipped, never causing the decoder to abort.

### Key Components
- **StreamEvent**: Top-level event with discriminated `Type` field (system, assistant, user, result)
- **ContentBlock**: Polymorphic content block (text, tool_use, tool_result) within messages
- **StreamDecoder**: JSONL line reader with `Next()` iterator and `Decode()` channel producer
- **Helper methods**: `ToolUseBlocks()`, `TextContent()`, `IsToolUse()`, `IsText()` for ergonomic consumption

### API/Interface Contracts
```go
// internal/agent/stream.go
package agent

import (
    "bufio"
    "context"
    "encoding/json"
    "io"
)

// StreamEventType identifies the type of a stream-json event from Claude Code.
type StreamEventType string

const (
    StreamEventSystem    StreamEventType = "system"
    StreamEventAssistant StreamEventType = "assistant"
    StreamEventUser      StreamEventType = "user"
    StreamEventResult    StreamEventType = "result"
)

// StreamEvent represents a single JSONL event from Claude Code's
// --output-format stream-json output.
type StreamEvent struct {
    Type      StreamEventType `json:"type"`
    Subtype   string          `json:"subtype,omitempty"`
    SessionID string          `json:"session_id,omitempty"`

    // System init fields
    Tools      []string `json:"tools,omitempty"`
    MCPServers []string `json:"mcp_servers,omitempty"`
    Model      string   `json:"model,omitempty"`

    // Message fields (for assistant and user events)
    Message *StreamMessage `json:"message,omitempty"`

    // Result fields
    CostUSD       float64 `json:"cost_usd,omitempty"`
    DurationMS    int64   `json:"duration_ms,omitempty"`
    DurationAPIMS int64   `json:"duration_api_ms,omitempty"`
    IsError       bool    `json:"is_error,omitempty"`
    NumTurns      int     `json:"num_turns,omitempty"`
}

// StreamMessage represents a message within a stream event.
type StreamMessage struct {
    ID         string         `json:"id,omitempty"`
    Type       string         `json:"type,omitempty"`
    Role       string         `json:"role,omitempty"`
    Content    []ContentBlock `json:"content,omitempty"`
    Model      string         `json:"model,omitempty"`
    StopReason string         `json:"stop_reason,omitempty"`
    Usage      *StreamUsage   `json:"usage,omitempty"`
}

// ContentBlock represents a content block within a message.
// The Type field determines which other fields are populated.
type ContentBlock struct {
    Type      string          `json:"type"`
    Text      string          `json:"text,omitempty"`
    ID        string          `json:"id,omitempty"`
    Name      string          `json:"name,omitempty"`
    Input     json.RawMessage `json:"input,omitempty"`
    ToolUseID string          `json:"tool_use_id,omitempty"`
    Content   json.RawMessage `json:"content,omitempty"`
}

// StreamUsage captures token usage from a stream event.
type StreamUsage struct {
    InputTokens  int `json:"input_tokens"`
    OutputTokens int `json:"output_tokens"`
    CacheRead    int `json:"cache_read_input_tokens,omitempty"`
    CacheCreate  int `json:"cache_creation_input_tokens,omitempty"`
}

// StreamDecoder reads JSONL events from an io.Reader.
type StreamDecoder struct {
    scanner *bufio.Scanner
}

// NewStreamDecoder creates a decoder that reads JSONL from r.
func NewStreamDecoder(r io.Reader) *StreamDecoder

// Next reads and decodes the next stream event.
// Returns the event and nil on success, or nil and io.EOF at end of stream.
// Malformed lines return a non-nil error (callers should log and continue).
func (d *StreamDecoder) Next() (*StreamEvent, error)

// Decode reads all events and sends them to the channel.
// Blocks until the reader is exhausted, context is cancelled, or a read error occurs.
// Closes the events channel when done. Malformed lines are skipped.
func (d *StreamDecoder) Decode(ctx context.Context, events chan<- StreamEvent) error

// --- Helper methods on StreamEvent ---

// ToolUseBlocks returns all tool_use content blocks from this event's message.
func (e *StreamEvent) ToolUseBlocks() []ContentBlock

// TextContent returns concatenated text from all text content blocks.
func (e *StreamEvent) TextContent() string

// --- Helper methods on ContentBlock ---

// IsText returns true if the content block is a text block.
func (b *ContentBlock) IsText() bool

// IsToolUse returns true if the content block is a tool_use block.
func (b *ContentBlock) IsToolUse() bool

// IsToolResult returns true if the content block is a tool_result block.
func (b *ContentBlock) IsToolResult() bool

// InputString returns the tool input as a formatted string.
func (b *ContentBlock) InputString() string

// ContentString returns the tool result content as a string.
func (b *ContentBlock) ContentString() string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| encoding/json | stdlib | JSONL decoding |
| bufio | stdlib | Line-by-line reading |
| io | stdlib | Reader interface |
| context | stdlib | Cancellation for async decode |
| strings | stdlib | Text concatenation |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] StreamEvent types map faithfully to Claude Code's stream-json output format
- [ ] StreamDecoder.Next() reads one JSONL line and returns a typed StreamEvent
- [ ] StreamDecoder.Next() returns io.EOF when the reader is exhausted
- [ ] StreamDecoder.Next() returns an error for malformed JSON (caller can skip)
- [ ] StreamDecoder.Decode() sends events to channel until reader exhausted
- [ ] StreamDecoder.Decode() respects context cancellation
- [ ] StreamDecoder.Decode() closes the events channel when done
- [ ] StreamDecoder.Decode() skips malformed lines without aborting
- [ ] ToolUseBlocks() extracts tool_use content blocks from assistant events
- [ ] TextContent() concatenates text blocks from assistant events
- [ ] ContentBlock helpers (IsText, IsToolUse, IsToolResult) work correctly
- [ ] ContentBlock.InputString() returns formatted tool input
- [ ] ContentBlock.ContentString() returns tool result as string
- [ ] Empty/nil message in event returns empty results from helpers (no panic)
- [ ] Scanner buffer handles very long lines (>64KB tool results)
- [ ] Unit tests achieve 90% coverage
- [ ] All types serialize/deserialize with encoding/json round-trip

## Testing Requirements
### Unit Tests (Table-Driven)
- Decode system init event: tools, model, session_id populated
- Decode assistant event with text block: TextContent() returns text
- Decode assistant event with tool_use block: ToolUseBlocks() returns block with name and input
- Decode user event with tool_result block: ContentString() returns result
- Decode result event: CostUSD, DurationMS, NumTurns populated
- Decode event with multiple content blocks: both text and tool_use in same message
- Decode event with usage stats: InputTokens, OutputTokens populated
- Malformed JSON line: Next() returns error, not panic
- Empty line: Next() skips and reads next line
- io.EOF at end of stream: Next() returns io.EOF
- Decode() with context cancellation: stops and closes channel
- Decode() with 3 events: channel receives all 3 then closes
- Decode() with malformed line in middle: skips bad line, delivers others
- ContentBlock.IsText() / IsToolUse() / IsToolResult(): correct for each type
- ToolUseBlocks() on non-assistant event: returns empty slice
- TextContent() on event with no text blocks: returns empty string
- InputString() with complex JSON input: returns formatted string
- ContentString() with string content: returns the string
- ContentString() with structured content: returns JSON string
- JSON round-trip: marshal then unmarshal preserves all fields

### Golden Tests
- Full session JSONL fixture (system + assistant + user + result): decode all events and verify

### Edge Cases to Handle
- Very long tool result content (>64KB): scanner buffer must be large enough
- Unicode in text content: preserved through decode
- Nested JSON in tool input: preserved as json.RawMessage
- Empty content array in message: helpers return zero values
- Nil message in event: helpers return zero values (no nil pointer panic)
- Blank lines between events: skipped gracefully
- Trailing whitespace on lines: handled by json.Unmarshal

## Implementation Notes
### Recommended Approach
1. Define types first (StreamEvent, StreamMessage, ContentBlock, StreamUsage)
2. Implement NewStreamDecoder with bufio.Scanner and enlarged buffer (1MB)
3. Implement Next() -- scan line, skip empty, unmarshal
4. Implement Decode() -- loop calling Next(), send to channel with ctx select
5. Implement helper methods on StreamEvent and ContentBlock
6. Write testdata fixtures with real Claude Code JSONL output samples
7. Table-driven tests for each event type and edge case

### Impact on Existing Tasks
- **T-022 (Claude Agent Adapter)**: Should use `--output-format stream-json` instead of `--output-format json` when streaming is requested. The adapter's `Run()` method gains a `StreamEvents` channel in RunOpts.
- **T-027 (Implementation Loop)**: Can consume StreamEvents to emit fine-grained LoopEvents (tool started, tool completed) instead of only high-level events.
- **T-067 (TUI Messages)**: Gains new message types for tool call display (ToolCallMsg, ToolResultMsg) mapped from StreamEvent content blocks.
- **T-073 (Agent Output Panel)**: Can render tool calls, reasoning, and results in separate tabs/sections.

### Potential Pitfalls
- The scanner's default 64KB buffer is too small for tool results. Use `scanner.Buffer(buf, maxSize)` with a 1MB buffer.
- `json.RawMessage` fields (`Input`, `Content`) must be handled carefully in tests -- use `json.Marshal` for comparison, not string equality.
- The Decode() method must close the channel even on error/cancellation to prevent consumer goroutine leaks.
- Context cancellation in Decode() should drain any in-flight scan before returning.

### Security Considerations
- Stream events may contain sensitive source code in tool inputs/results. Log at debug level only.
- Session IDs should not be exposed in user-facing TUI -- they are internal correlation IDs.

## References
- [Claude Code CLI --output-format documentation](https://docs.anthropic.com/en/docs/claude-code/cli-usage)
- [Claude Code Hooks documentation](https://docs.anthropic.com/en/docs/claude-code/hooks)
- [Go encoding/json streaming](https://pkg.go.dev/encoding/json#Decoder)
- [JSONL specification](https://jsonlines.org/)
- [PRD Section 5.2 - Agent Adapter System](docs/prd/PRD-Raven.md)
- [PRD Section 10 Q3 - CLI vs API decision](docs/prd/PRD-Raven.md)
