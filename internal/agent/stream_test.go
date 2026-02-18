package agent

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamEventType_Constants(t *testing.T) {
	assert.Equal(t, StreamEventType("system"), StreamEventSystem)
	assert.Equal(t, StreamEventType("assistant"), StreamEventAssistant)
	assert.Equal(t, StreamEventType("user"), StreamEventUser)
	assert.Equal(t, StreamEventType("result"), StreamEventResult)
}

func TestStreamDecoder_Next(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantType  StreamEventType
		wantErr   bool
		wantEOF   bool
		checkFunc func(t *testing.T, event *StreamEvent)
	}{
		{
			name:     "system init event",
			input:    `{"type":"system","subtype":"init","session_id":"sess_123","tools":["Read","Edit","Bash"],"mcp_servers":["fs"],"model":"claude-opus-4-6"}`,
			wantType: StreamEventSystem,
			checkFunc: func(t *testing.T, event *StreamEvent) {
				assert.Equal(t, "init", event.Subtype)
				assert.Equal(t, "sess_123", event.SessionID)
				assert.Equal(t, []string{"Read", "Edit", "Bash"}, event.Tools)
				assert.Equal(t, []string{"fs"}, event.MCPServers)
				assert.Equal(t, "claude-opus-4-6", event.Model)
			},
		},
		{
			name:     "assistant event with text",
			input:    `{"type":"assistant","message":{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"Hello world"}],"model":"claude-opus-4-6","stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":5}}}`,
			wantType: StreamEventAssistant,
			checkFunc: func(t *testing.T, event *StreamEvent) {
				require.NotNil(t, event.Message)
				assert.Equal(t, "msg_1", event.Message.ID)
				assert.Equal(t, "assistant", event.Message.Role)
				assert.Equal(t, "end_turn", event.Message.StopReason)
				require.Len(t, event.Message.Content, 1)
				assert.True(t, event.Message.Content[0].IsText())
				assert.Equal(t, "Hello world", event.Message.Content[0].Text)
				require.NotNil(t, event.Message.Usage)
				assert.Equal(t, 100, event.Message.Usage.InputTokens)
				assert.Equal(t, 5, event.Message.Usage.OutputTokens)
			},
		},
		{
			name:     "assistant event with tool_use",
			input:    `{"type":"assistant","message":{"id":"msg_2","role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"/tmp/test.go"}}],"stop_reason":"tool_use"}}`,
			wantType: StreamEventAssistant,
			checkFunc: func(t *testing.T, event *StreamEvent) {
				require.NotNil(t, event.Message)
				require.Len(t, event.Message.Content, 1)
				block := event.Message.Content[0]
				assert.True(t, block.IsToolUse())
				assert.False(t, block.IsText())
				assert.False(t, block.IsToolResult())
				assert.Equal(t, "toolu_1", block.ID)
				assert.Equal(t, "Read", block.Name)
				assert.Contains(t, block.InputString(), "/tmp/test.go")
			},
		},
		{
			name:     "user event with tool_result string content",
			input:    `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"file contents here"}]}}`,
			wantType: StreamEventUser,
			checkFunc: func(t *testing.T, event *StreamEvent) {
				require.NotNil(t, event.Message)
				require.Len(t, event.Message.Content, 1)
				block := event.Message.Content[0]
				assert.True(t, block.IsToolResult())
				assert.Equal(t, "toolu_1", block.ToolUseID)
				assert.Equal(t, "file contents here", block.ContentString())
			},
		},
		{
			name:     "user event with tool_result structured content",
			input:    `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_2","content":{"status":"ok","lines":42}}]}}`,
			wantType: StreamEventUser,
			checkFunc: func(t *testing.T, event *StreamEvent) {
				require.NotNil(t, event.Message)
				require.Len(t, event.Message.Content, 1)
				block := event.Message.Content[0]
				assert.True(t, block.IsToolResult())
				// Structured content returns raw JSON.
				content := block.ContentString()
				assert.Contains(t, content, "status")
				assert.Contains(t, content, "ok")
			},
		},
		{
			name:     "result event success",
			input:    `{"type":"result","subtype":"success","session_id":"sess_123","cost_usd":0.042,"duration_ms":15230,"duration_api_ms":12100,"is_error":false,"num_turns":3}`,
			wantType: StreamEventResult,
			checkFunc: func(t *testing.T, event *StreamEvent) {
				assert.Equal(t, "success", event.Subtype)
				assert.Equal(t, "sess_123", event.SessionID)
				assert.InDelta(t, 0.042, event.CostUSD, 0.0001)
				assert.Equal(t, int64(15230), event.DurationMS)
				assert.Equal(t, int64(12100), event.DurationAPIMS)
				assert.False(t, event.IsError)
				assert.Equal(t, 3, event.NumTurns)
			},
		},
		{
			name:     "result event error",
			input:    `{"type":"result","subtype":"error","is_error":true,"num_turns":1}`,
			wantType: StreamEventResult,
			checkFunc: func(t *testing.T, event *StreamEvent) {
				assert.Equal(t, "error", event.Subtype)
				assert.True(t, event.IsError)
				assert.Equal(t, 1, event.NumTurns)
			},
		},
		{
			name:    "malformed JSON",
			input:   `this is not valid json`,
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   ``,
			wantEOF: true,
		},
		{
			name:     "skip blank lines",
			input:    "\n\n" + `{"type":"system","subtype":"init","session_id":"s1"}` + "\n\n",
			wantType: StreamEventSystem,
			checkFunc: func(t *testing.T, event *StreamEvent) {
				assert.Equal(t, "s1", event.SessionID)
			},
		},
		{
			name:     "assistant with mixed text and tool_use",
			input:    `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Let me check "},{"type":"text","text":"the file."},{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/a.go"}}]}}`,
			wantType: StreamEventAssistant,
			checkFunc: func(t *testing.T, event *StreamEvent) {
				assert.Equal(t, "Let me check the file.", event.TextContent())
				toolBlocks := event.ToolUseBlocks()
				require.Len(t, toolBlocks, 1)
				assert.Equal(t, "Read", toolBlocks[0].Name)
			},
		},
		{
			name:     "usage with cache tokens",
			input:    `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":500,"output_tokens":10,"cache_read_input_tokens":300,"cache_creation_input_tokens":50}}}`,
			wantType: StreamEventAssistant,
			checkFunc: func(t *testing.T, event *StreamEvent) {
				require.NotNil(t, event.Message.Usage)
				assert.Equal(t, 500, event.Message.Usage.InputTokens)
				assert.Equal(t, 10, event.Message.Usage.OutputTokens)
				assert.Equal(t, 300, event.Message.Usage.CacheRead)
				assert.Equal(t, 50, event.Message.Usage.CacheCreate)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := NewStreamDecoder(strings.NewReader(tt.input))
			event, err := dec.Next()

			if tt.wantEOF {
				assert.ErrorIs(t, err, io.EOF)
				assert.Nil(t, event)
				return
			}
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, event)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, event)
			assert.Equal(t, tt.wantType, event.Type)
			if tt.checkFunc != nil {
				tt.checkFunc(t, event)
			}
		})
	}
}

func TestStreamDecoder_Next_MultipleEvents(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"s1"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}`,
		`{"type":"result","subtype":"success","num_turns":1}`,
	}, "\n")

	dec := NewStreamDecoder(strings.NewReader(input))

	event1, err := dec.Next()
	require.NoError(t, err)
	assert.Equal(t, StreamEventSystem, event1.Type)

	event2, err := dec.Next()
	require.NoError(t, err)
	assert.Equal(t, StreamEventAssistant, event2.Type)

	event3, err := dec.Next()
	require.NoError(t, err)
	assert.Equal(t, StreamEventResult, event3.Type)

	_, err = dec.Next()
	assert.ErrorIs(t, err, io.EOF)
}

func TestStreamDecoder_Decode(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"s1"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"working"}]}}`,
		`{"type":"result","subtype":"success","num_turns":1}`,
	}, "\n")

	dec := NewStreamDecoder(strings.NewReader(input))
	ch := make(chan StreamEvent, 10)
	ctx := context.Background()

	err := dec.Decode(ctx, ch)
	require.NoError(t, err)

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	require.Len(t, events, 3)
	assert.Equal(t, StreamEventSystem, events[0].Type)
	assert.Equal(t, StreamEventAssistant, events[1].Type)
	assert.Equal(t, StreamEventResult, events[2].Type)
}

func TestStreamDecoder_Decode_ContextCancellation(t *testing.T) {
	// Create a reader that blocks indefinitely.
	r, w := io.Pipe()
	defer w.Close()

	// Write one event to make progress, then block.
	go func() {
		_, _ = w.Write([]byte(`{"type":"system","subtype":"init","session_id":"s1"}` + "\n"))
		// Don't write more -- reader will block on next line.
	}()

	dec := NewStreamDecoder(r)
	ch := make(chan StreamEvent, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Decode should return context error after cancellation.
	// Close the pipe to unblock the scanner when context is cancelled.
	go func() {
		<-ctx.Done()
		w.Close()
	}()

	err := dec.Decode(ctx, ch)
	// Either context.DeadlineExceeded or nil (if pipe closed first).
	if err != nil {
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	}

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		// Got the system event; next read should indicate closed.
		_, ok = <-ch
		// Channel may or may not have more; eventually it closes.
	}
	// We mainly verify Decode returns and doesn't hang.
}

func TestStreamDecoder_Decode_SkipsMalformedLines(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"s1"}`,
		`not valid json at all`,
		`{"type":"result","subtype":"success","num_turns":1}`,
	}, "\n")

	dec := NewStreamDecoder(strings.NewReader(input))
	ch := make(chan StreamEvent, 10)

	err := dec.Decode(context.Background(), ch)
	require.NoError(t, err)

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	// Malformed line is skipped; we get 2 events.
	require.Len(t, events, 2)
	assert.Equal(t, StreamEventSystem, events[0].Type)
	assert.Equal(t, StreamEventResult, events[1].Type)
}

func TestStreamDecoder_Decode_EmptyInput(t *testing.T) {
	dec := NewStreamDecoder(strings.NewReader(""))
	ch := make(chan StreamEvent, 10)

	err := dec.Decode(context.Background(), ch)
	require.NoError(t, err)

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	assert.Empty(t, events)
}

func TestStreamEvent_ToolUseBlocks(t *testing.T) {
	tests := []struct {
		name  string
		event StreamEvent
		want  int
	}{
		{
			name:  "nil message",
			event: StreamEvent{Type: StreamEventSystem},
			want:  0,
		},
		{
			name: "no tool_use blocks",
			event: StreamEvent{
				Type: StreamEventAssistant,
				Message: &StreamMessage{
					Content: []ContentBlock{
						{Type: "text", Text: "hello"},
					},
				},
			},
			want: 0,
		},
		{
			name: "one tool_use block",
			event: StreamEvent{
				Type: StreamEventAssistant,
				Message: &StreamMessage{
					Content: []ContentBlock{
						{Type: "text", Text: "let me check"},
						{Type: "tool_use", Name: "Read", ID: "t1"},
					},
				},
			},
			want: 1,
		},
		{
			name: "multiple tool_use blocks",
			event: StreamEvent{
				Type: StreamEventAssistant,
				Message: &StreamMessage{
					Content: []ContentBlock{
						{Type: "tool_use", Name: "Read", ID: "t1"},
						{Type: "tool_use", Name: "Grep", ID: "t2"},
					},
				},
			},
			want: 2,
		},
		{
			name: "empty content",
			event: StreamEvent{
				Type:    StreamEventAssistant,
				Message: &StreamMessage{Content: []ContentBlock{}},
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.event.ToolUseBlocks()
			assert.Len(t, got, tt.want)
		})
	}
}

func TestStreamEvent_TextContent(t *testing.T) {
	tests := []struct {
		name  string
		event StreamEvent
		want  string
	}{
		{
			name:  "nil message",
			event: StreamEvent{Type: StreamEventSystem},
			want:  "",
		},
		{
			name: "single text block",
			event: StreamEvent{
				Type: StreamEventAssistant,
				Message: &StreamMessage{
					Content: []ContentBlock{
						{Type: "text", Text: "hello world"},
					},
				},
			},
			want: "hello world",
		},
		{
			name: "multiple text blocks concatenated",
			event: StreamEvent{
				Type: StreamEventAssistant,
				Message: &StreamMessage{
					Content: []ContentBlock{
						{Type: "text", Text: "part one "},
						{Type: "tool_use", Name: "Read"},
						{Type: "text", Text: "part two"},
					},
				},
			},
			want: "part one part two",
		},
		{
			name: "no text blocks",
			event: StreamEvent{
				Type: StreamEventAssistant,
				Message: &StreamMessage{
					Content: []ContentBlock{
						{Type: "tool_use", Name: "Read"},
					},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.event.TextContent())
		})
	}
}

func TestStreamEvent_ToolResultBlocks(t *testing.T) {
	event := StreamEvent{
		Type: StreamEventUser,
		Message: &StreamMessage{
			Content: []ContentBlock{
				{Type: "tool_result", ToolUseID: "t1", Content: json.RawMessage(`"ok"`)},
				{Type: "tool_result", ToolUseID: "t2", Content: json.RawMessage(`"done"`)},
			},
		},
	}
	results := event.ToolResultBlocks()
	assert.Len(t, results, 2)
	assert.Equal(t, "t1", results[0].ToolUseID)
	assert.Equal(t, "t2", results[1].ToolUseID)
}

func TestContentBlock_Helpers(t *testing.T) {
	text := ContentBlock{Type: "text", Text: "hello"}
	assert.True(t, text.IsText())
	assert.False(t, text.IsToolUse())
	assert.False(t, text.IsToolResult())

	toolUse := ContentBlock{Type: "tool_use", Name: "Bash", ID: "t1"}
	assert.False(t, toolUse.IsText())
	assert.True(t, toolUse.IsToolUse())
	assert.False(t, toolUse.IsToolResult())

	toolResult := ContentBlock{Type: "tool_result", ToolUseID: "t1"}
	assert.False(t, toolResult.IsText())
	assert.False(t, toolResult.IsToolUse())
	assert.True(t, toolResult.IsToolResult())
}

func TestContentBlock_InputString(t *testing.T) {
	tests := []struct {
		name  string
		input json.RawMessage
		want  string
	}{
		{
			name:  "nil input",
			input: nil,
			want:  "",
		},
		{
			name:  "empty input",
			input: json.RawMessage{},
			want:  "",
		},
		{
			name:  "simple object",
			input: json.RawMessage(`{"file_path":"/tmp/test.go"}`),
			want:  "{\n  \"file_path\": \"/tmp/test.go\"\n}",
		},
		{
			name:  "complex nested object",
			input: json.RawMessage(`{"command":"go test ./...","timeout":30}`),
			want:  "{\n  \"command\": \"go test ./...\",\n  \"timeout\": 30\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := ContentBlock{Type: "tool_use", Input: tt.input}
			assert.Equal(t, tt.want, b.InputString())
		})
	}
}

func TestContentBlock_ContentString(t *testing.T) {
	tests := []struct {
		name    string
		content json.RawMessage
		want    string
	}{
		{
			name:    "nil content",
			content: nil,
			want:    "",
		},
		{
			name:    "empty content",
			content: json.RawMessage{},
			want:    "",
		},
		{
			name:    "string content",
			content: json.RawMessage(`"file contents here"`),
			want:    "file contents here",
		},
		{
			name:    "structured content",
			content: json.RawMessage(`{"status":"ok","lines":42}`),
			want:    `{"status":"ok","lines":42}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := ContentBlock{Type: "tool_result", Content: tt.content}
			assert.Equal(t, tt.want, b.ContentString())
		})
	}
}

func TestStreamEvent_JSONRoundTrip(t *testing.T) {
	original := StreamEvent{
		Type:      StreamEventAssistant,
		SessionID: "sess_rt",
		Message: &StreamMessage{
			ID:         "msg_rt",
			Role:       "assistant",
			StopReason: "tool_use",
			Content: []ContentBlock{
				{Type: "text", Text: "reasoning here"},
				{Type: "tool_use", ID: "t1", Name: "Read", Input: json.RawMessage(`{"file_path":"/a.go"}`)},
			},
			Usage: &StreamUsage{
				InputTokens:  1000,
				OutputTokens: 50,
				CacheRead:    200,
			},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded StreamEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.Type, decoded.Type)
	assert.Equal(t, original.SessionID, decoded.SessionID)
	require.NotNil(t, decoded.Message)
	assert.Equal(t, original.Message.ID, decoded.Message.ID)
	assert.Equal(t, original.Message.Role, decoded.Message.Role)
	require.Len(t, decoded.Message.Content, 2)
	assert.Equal(t, "text", decoded.Message.Content[0].Type)
	assert.Equal(t, "reasoning here", decoded.Message.Content[0].Text)
	assert.Equal(t, "tool_use", decoded.Message.Content[1].Type)
	assert.Equal(t, "Read", decoded.Message.Content[1].Name)
	require.NotNil(t, decoded.Message.Usage)
	assert.Equal(t, 1000, decoded.Message.Usage.InputTokens)
	assert.Equal(t, 200, decoded.Message.Usage.CacheRead)
}

func TestStreamDecoder_GoldenFullSession(t *testing.T) {
	f, err := os.Open("../../testdata/stream-json/session-full.jsonl")
	if err != nil {
		t.Skip("testdata fixture not found:", err)
	}
	defer f.Close()

	dec := NewStreamDecoder(f)
	var events []StreamEvent
	for {
		event, err := dec.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		events = append(events, *event)
	}

	require.Len(t, events, 8)

	// Event 0: system init.
	assert.Equal(t, StreamEventSystem, events[0].Type)
	assert.Equal(t, "init", events[0].Subtype)
	assert.Equal(t, "sess_abc123", events[0].SessionID)
	assert.Equal(t, "claude-opus-4-6", events[0].Model)
	assert.Contains(t, events[0].Tools, "Read")
	assert.Contains(t, events[0].Tools, "Edit")
	assert.Contains(t, events[0].MCPServers, "filesystem")

	// Event 1: assistant text.
	assert.Equal(t, StreamEventAssistant, events[1].Type)
	assert.Contains(t, events[1].TextContent(), "read the main configuration file")

	// Event 2: assistant tool_use (Read).
	assert.Equal(t, StreamEventAssistant, events[2].Type)
	toolBlocks := events[2].ToolUseBlocks()
	require.Len(t, toolBlocks, 1)
	assert.Equal(t, "Read", toolBlocks[0].Name)
	assert.Contains(t, toolBlocks[0].InputString(), "config.toml")

	// Event 3: user tool_result.
	assert.Equal(t, StreamEventUser, events[3].Type)
	results := events[3].ToolResultBlocks()
	require.Len(t, results, 1)
	assert.Contains(t, results[0].ContentString(), "port = 8080")

	// Event 4: assistant text + tool_use (Edit).
	assert.Equal(t, StreamEventAssistant, events[4].Type)
	assert.Contains(t, events[4].TextContent(), "update the port")
	editBlocks := events[4].ToolUseBlocks()
	require.Len(t, editBlocks, 1)
	assert.Equal(t, "Edit", editBlocks[0].Name)

	// Event 5: user tool_result (Edit success).
	assert.Equal(t, StreamEventUser, events[5].Type)

	// Event 6: assistant final text.
	assert.Equal(t, StreamEventAssistant, events[6].Type)
	assert.Contains(t, events[6].TextContent(), "updated")

	// Event 7: result.
	assert.Equal(t, StreamEventResult, events[7].Type)
	assert.Equal(t, "success", events[7].Subtype)
	assert.InDelta(t, 0.042, events[7].CostUSD, 0.0001)
	assert.Equal(t, int64(15230), events[7].DurationMS)
	assert.Equal(t, 3, events[7].NumTurns)
	assert.False(t, events[7].IsError)
}

func TestStreamDecoder_GoldenErrorSession(t *testing.T) {
	f, err := os.Open("../../testdata/stream-json/session-error.jsonl")
	if err != nil {
		t.Skip("testdata fixture not found:", err)
	}
	defer f.Close()

	dec := NewStreamDecoder(f)
	var events []StreamEvent
	for {
		event, err := dec.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		events = append(events, *event)
	}

	require.Len(t, events, 3)
	assert.Equal(t, StreamEventResult, events[2].Type)
	assert.True(t, events[2].IsError)
	assert.Equal(t, "error", events[2].Subtype)
}

func TestStreamDecoder_GoldenMalformedMixed(t *testing.T) {
	f, err := os.Open("../../testdata/stream-json/malformed-mixed.jsonl")
	if err != nil {
		t.Skip("testdata fixture not found:", err)
	}
	defer f.Close()

	dec := NewStreamDecoder(f)
	ch := make(chan StreamEvent, 10)

	err = dec.Decode(context.Background(), ch)
	require.NoError(t, err)

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	// Should get 3 valid events (malformed line skipped, blank line skipped).
	require.Len(t, events, 3)
	assert.Equal(t, StreamEventSystem, events[0].Type)
	assert.Equal(t, StreamEventAssistant, events[1].Type)
	assert.Equal(t, StreamEventResult, events[2].Type)
}

func TestStreamEvent_Unicode(t *testing.T) {
	input := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"日本語テスト: 🎉 émojis and ñ"}]}}`

	dec := NewStreamDecoder(strings.NewReader(input))
	event, err := dec.Next()
	require.NoError(t, err)
	assert.Equal(t, "日本語テスト: 🎉 émojis and ñ", event.TextContent())
}

func TestStreamEvent_LargeToolResult(t *testing.T) {
	// Simulate a large tool result (100KB).
	largeContent := strings.Repeat("x", 100*1024)
	input := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"` + largeContent + `"}]}}`

	dec := NewStreamDecoder(strings.NewReader(input))
	event, err := dec.Next()
	require.NoError(t, err)
	results := event.ToolResultBlocks()
	require.Len(t, results, 1)
	assert.Len(t, results[0].ContentString(), 100*1024)
}
