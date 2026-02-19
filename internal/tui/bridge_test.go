package tui

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/loop"
	"github.com/AbdelazizMoustafa10m/Raven/internal/workflow"
)

// TestNewEventBridge verifies that NewEventBridge returns a usable EventBridge.
func TestNewEventBridge(t *testing.T) {
	t.Parallel()
	b := NewEventBridge()
	assert.NotNil(t, b)
}

// TestEventBridge_WorkflowEventCmd_ReceivesEvent verifies that the returned
// tea.Cmd converts a workflow.WorkflowEvent to a WorkflowEventMsg.
func TestEventBridge_WorkflowEventCmd_ReceivesEvent(t *testing.T) {
	t.Parallel()

	b := NewEventBridge()
	ch := make(chan workflow.WorkflowEvent, 1)

	ts := time.Now()
	ch <- workflow.WorkflowEvent{
		WorkflowID: "wf-1",
		Step:       "implement",
		Event:      "success",
		Message:    "step done",
		Timestamp:  ts,
	}

	ctx := context.Background()
	cmd := b.WorkflowEventCmd(ctx, ch)
	require.NotNil(t, cmd)

	msg := cmd()
	wfMsg, ok := msg.(WorkflowEventMsg)
	require.True(t, ok, "expected WorkflowEventMsg, got %T", msg)

	assert.Equal(t, "wf-1", wfMsg.WorkflowID)
	assert.Equal(t, "implement", wfMsg.Step)
	assert.Equal(t, "success", wfMsg.Event)
	assert.Equal(t, "step done", wfMsg.Detail)
	assert.Equal(t, ts, wfMsg.Timestamp)
}

// TestEventBridge_WorkflowEventCmd_ClosedChannel verifies that the command
// returns nil when the channel is closed.
func TestEventBridge_WorkflowEventCmd_ClosedChannel(t *testing.T) {
	t.Parallel()

	b := NewEventBridge()
	ch := make(chan workflow.WorkflowEvent)
	close(ch)

	ctx := context.Background()
	cmd := b.WorkflowEventCmd(ctx, ch)
	require.NotNil(t, cmd)

	msg := cmd()
	assert.Nil(t, msg)
}

// TestEventBridge_WorkflowEventCmd_CancelledContext verifies that the command
// returns nil when the context is cancelled.
func TestEventBridge_WorkflowEventCmd_CancelledContext(t *testing.T) {
	t.Parallel()

	b := NewEventBridge()
	ch := make(chan workflow.WorkflowEvent) // never receives

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cmd := b.WorkflowEventCmd(ctx, ch)
	require.NotNil(t, cmd)

	msg := cmd()
	assert.Nil(t, msg)
}

// TestMapLoopEventType_AllTypes verifies the mapping from loop.LoopEventType
// to tui.LoopEventType for all defined loop event type constants.
func TestMapLoopEventType_AllTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  loop.LoopEventType
		expect LoopEventType
	}{
		{name: "task_selected", input: loop.EventTaskSelected, expect: LoopTaskSelected},
		{name: "task_completed", input: loop.EventTaskCompleted, expect: LoopTaskCompleted},
		{name: "task_blocked", input: loop.EventTaskBlocked, expect: LoopTaskBlocked},
		{name: "rate_limit_resume", input: loop.EventRateLimitResume, expect: LoopResumedAfterWait},
		{name: "phase_complete", input: loop.EventPhaseComplete, expect: LoopPhaseComplete},
		{name: "loop_error", input: loop.EventLoopError, expect: LoopError},
		{name: "loop_aborted", input: loop.EventLoopAborted, expect: LoopError},
		{name: "agent_started", input: loop.EventAgentStarted, expect: LoopIterationStarted},
		{name: "loop_started", input: loop.EventLoopStarted, expect: LoopIterationStarted},
		{name: "agent_completed", input: loop.EventAgentCompleted, expect: LoopIterationCompleted},
		{name: "unknown_defaults", input: loop.LoopEventType("unknown_type"), expect: LoopIterationStarted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mapLoopEventType(tt.input)
			assert.Equal(t, tt.expect, got)
		})
	}
}

// TestEventBridge_LoopEventCmd_RateLimitConvertsToRateLimitMsg verifies that
// rate-limit loop events are converted to RateLimitMsg.
func TestEventBridge_LoopEventCmd_RateLimitConvertsToRateLimitMsg(t *testing.T) {
	t.Parallel()

	b := NewEventBridge()
	ch := make(chan loop.LoopEvent, 1)

	ts := time.Now()
	ch <- loop.LoopEvent{
		Type:      loop.EventRateLimitWait,
		AgentName: "claude",
		WaitTime:  30 * time.Second,
		Timestamp: ts,
	}

	ctx := context.Background()
	cmd := b.LoopEventCmd(ctx, ch)
	require.NotNil(t, cmd)

	msg := cmd()
	rlMsg, ok := msg.(RateLimitMsg)
	require.True(t, ok, "expected RateLimitMsg for rate-limit event, got %T", msg)

	assert.Equal(t, "claude", rlMsg.Agent)
	assert.Equal(t, "claude", rlMsg.Provider)
	assert.Equal(t, 30*time.Second, rlMsg.ResetAfter)
}

// TestEventBridge_LoopEventCmd_NormalEventConvertsToLoopEventMsg verifies that
// normal loop events are converted to LoopEventMsg.
func TestEventBridge_LoopEventCmd_NormalEventConvertsToLoopEventMsg(t *testing.T) {
	t.Parallel()

	b := NewEventBridge()
	ch := make(chan loop.LoopEvent, 1)

	ts := time.Now()
	ch <- loop.LoopEvent{
		Type:      loop.EventTaskCompleted,
		TaskID:    "T-001",
		Iteration: 3,
		Message:   "task done",
		Timestamp: ts,
	}

	ctx := context.Background()
	cmd := b.LoopEventCmd(ctx, ch)
	require.NotNil(t, cmd)

	msg := cmd()
	loopMsg, ok := msg.(LoopEventMsg)
	require.True(t, ok, "expected LoopEventMsg for normal event, got %T", msg)

	assert.Equal(t, LoopTaskCompleted, loopMsg.Type)
	assert.Equal(t, "T-001", loopMsg.TaskID)
	assert.Equal(t, 3, loopMsg.Iteration)
	assert.Equal(t, "task done", loopMsg.Detail)
}

// TestEventBridge_LoopEventCmd_ClosedChannel verifies that the command
// returns nil when the loop event channel is closed.
func TestEventBridge_LoopEventCmd_ClosedChannel(t *testing.T) {
	t.Parallel()

	b := NewEventBridge()
	ch := make(chan loop.LoopEvent)
	close(ch)

	ctx := context.Background()
	cmd := b.LoopEventCmd(ctx, ch)
	require.NotNil(t, cmd)

	msg := cmd()
	assert.Nil(t, msg)
}

// TestEventBridge_AgentOutputCmd_ReceivesMsg verifies that AgentOutputCmd
// forwards AgentOutputMsg values unchanged.
func TestEventBridge_AgentOutputCmd_ReceivesMsg(t *testing.T) {
	t.Parallel()

	b := NewEventBridge()
	ch := make(chan AgentOutputMsg, 1)

	ts := time.Now()
	ch <- AgentOutputMsg{
		Agent:     "claude",
		Line:      "hello world",
		Stream:    "stdout",
		Timestamp: ts,
	}

	ctx := context.Background()
	cmd := b.AgentOutputCmd(ctx, ch)
	require.NotNil(t, cmd)

	msg := cmd()
	aoMsg, ok := msg.(AgentOutputMsg)
	require.True(t, ok, "expected AgentOutputMsg, got %T", msg)

	assert.Equal(t, "claude", aoMsg.Agent)
	assert.Equal(t, "hello world", aoMsg.Line)
	assert.Equal(t, "stdout", aoMsg.Stream)
	assert.Equal(t, ts, aoMsg.Timestamp)
}

// TestEventBridge_TaskProgressCmd_ReceivesMsg verifies that TaskProgressCmd
// forwards TaskProgressMsg values unchanged.
func TestEventBridge_TaskProgressCmd_ReceivesMsg(t *testing.T) {
	t.Parallel()

	b := NewEventBridge()
	ch := make(chan TaskProgressMsg, 1)

	ts := time.Now()
	ch <- TaskProgressMsg{
		TaskID:    "T-001",
		TaskTitle: "first task",
		Status:    "completed",
		Phase:     1,
		Completed: 5,
		Total:     10,
		Timestamp: ts,
	}

	ctx := context.Background()
	cmd := b.TaskProgressCmd(ctx, ch)
	require.NotNil(t, cmd)

	msg := cmd()
	tpMsg, ok := msg.(TaskProgressMsg)
	require.True(t, ok, "expected TaskProgressMsg, got %T", msg)

	assert.Equal(t, "T-001", tpMsg.TaskID)
	assert.Equal(t, "completed", tpMsg.Status)
	assert.Equal(t, 5, tpMsg.Completed)
	assert.Equal(t, 10, tpMsg.Total)
}
