package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// StreamEventType identifies the type of a stream-json event from Claude Code.
type StreamEventType string

const (
	// StreamEventSystem is emitted once at session start with init metadata.
	StreamEventSystem StreamEventType = "system"
	// StreamEventAssistant contains assistant messages (text reasoning and tool calls).
	StreamEventAssistant StreamEventType = "assistant"
	// StreamEventUser contains tool results sent back to the model.
	StreamEventUser StreamEventType = "user"
	// StreamEventResult is emitted once at session end with cost and usage stats.
	StreamEventResult StreamEventType = "result"
)

// StreamEvent represents a single JSONL event from Claude Code's
// --output-format stream-json output. The Type field determines which
// other fields are populated.
type StreamEvent struct {
	Type      StreamEventType `json:"type"`
	Subtype   string          `json:"subtype,omitempty"`
	SessionID string          `json:"session_id,omitempty"`

	// System init fields (populated when Type == "system").
	Tools      []string `json:"tools,omitempty"`
	MCPServers []string `json:"mcp_servers,omitempty"`
	Model      string   `json:"model,omitempty"`

	// Message fields (populated when Type == "assistant" or "user").
	Message *StreamMessage `json:"message,omitempty"`

	// Result fields (populated when Type == "result").
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
// The Type field determines which other fields are populated:
//   - "text": Text field contains the reasoning text
//   - "tool_use": ID, Name, and Input fields describe the tool call
//   - "tool_result": ToolUseID and Content fields contain the tool output
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

// maxScannerBuffer is the maximum line length the decoder can handle (1MB).
// Claude Code tool results can be very large.
const maxScannerBuffer = 1 << 20

// StreamDecoder reads JSONL events from an io.Reader line-by-line.
type StreamDecoder struct {
	scanner *bufio.Scanner
}

// NewStreamDecoder creates a decoder that reads JSONL from r.
// The scanner buffer is sized to handle lines up to 1MB.
func NewStreamDecoder(r io.Reader) *StreamDecoder {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxScannerBuffer)
	return &StreamDecoder{scanner: scanner}
}

// Next reads and decodes the next stream event.
// Returns the event and nil on success, nil and io.EOF at end of stream,
// or nil and a decode error for malformed JSON lines.
// Empty and whitespace-only lines are skipped automatically.
func (d *StreamDecoder) Next() (*StreamEvent, error) {
	for d.scanner.Scan() {
		line := strings.TrimSpace(d.scanner.Text())
		if line == "" {
			continue
		}
		var event StreamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("decoding stream event: %w", err)
		}
		return &event, nil
	}
	if err := d.scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading stream: %w", err)
	}
	return nil, io.EOF
}

// Decode reads all events and sends them to the provided channel.
// It blocks until the reader is exhausted, context is cancelled, or a
// read error occurs. The events channel is closed when Decode returns.
// Malformed JSON lines are skipped silently.
func (d *StreamDecoder) Decode(ctx context.Context, events chan<- StreamEvent) error {
	defer close(events)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		event, err := d.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			// Skip malformed lines.
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case events <- *event:
		}
	}
}

// ToolUseBlocks returns all tool_use content blocks from this event's message.
// Returns nil if the event has no message or no tool_use blocks.
func (e *StreamEvent) ToolUseBlocks() []ContentBlock {
	if e.Message == nil {
		return nil
	}
	var blocks []ContentBlock
	for _, b := range e.Message.Content {
		if b.IsToolUse() {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// TextContent returns concatenated text from all text content blocks
// in this event's message. Returns an empty string if there are no
// text blocks or no message.
func (e *StreamEvent) TextContent() string {
	if e.Message == nil {
		return ""
	}
	var parts []string
	for _, b := range e.Message.Content {
		if b.IsText() {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "")
}

// ToolResultBlocks returns all tool_result content blocks from this event's message.
// Returns nil if the event has no message or no tool_result blocks.
func (e *StreamEvent) ToolResultBlocks() []ContentBlock {
	if e.Message == nil {
		return nil
	}
	var blocks []ContentBlock
	for _, b := range e.Message.Content {
		if b.IsToolResult() {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// IsText returns true if the content block is a text block.
func (b *ContentBlock) IsText() bool {
	return b.Type == "text"
}

// IsToolUse returns true if the content block is a tool_use block.
func (b *ContentBlock) IsToolUse() bool {
	return b.Type == "tool_use"
}

// IsToolResult returns true if the content block is a tool_result block.
func (b *ContentBlock) IsToolResult() bool {
	return b.Type == "tool_result"
}

// InputString returns the tool input as a formatted JSON string.
// Returns an empty string if the input is nil or empty.
func (b *ContentBlock) InputString() string {
	if len(b.Input) == 0 {
		return ""
	}
	// Try to pretty-print; fall back to raw.
	var v interface{}
	if err := json.Unmarshal(b.Input, &v); err != nil {
		return string(b.Input)
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(b.Input)
	}
	return string(pretty)
}

// ContentString returns the tool result content as a string.
// If the content is a JSON string, it is unquoted. Otherwise the
// raw JSON is returned. Returns an empty string if content is nil.
func (b *ContentBlock) ContentString() string {
	if len(b.Content) == 0 {
		return ""
	}
	// Try to decode as a JSON string first.
	var s string
	if err := json.Unmarshal(b.Content, &s); err == nil {
		return s
	}
	// Return raw JSON for structured content.
	return string(b.Content)
}
