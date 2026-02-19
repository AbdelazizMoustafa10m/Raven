package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AbdelazizMoustafa10m/Raven/internal/loop"
	"github.com/AbdelazizMoustafa10m/Raven/internal/workflow"
)

// EventBridge converts backend event types (workflow.WorkflowEvent,
// loop.LoopEvent) into TUI messages that the Bubble Tea runtime can dispatch
// to the App model. It is intended to be used as a tea.Cmd producer that reads
// from backend channels and forwards events into the Bubble Tea program.
//
// All methods are goroutine-safe: they spawn a background goroutine that reads
// from the given channel and returns a tea.Cmd that can be placed in a Batch.
// The goroutines respect the provided context for cancellation.
type EventBridge struct{}

// NewEventBridge creates a new EventBridge. No internal state is maintained;
// the struct exists to provide a namespaced API for the bridge helpers.
func NewEventBridge() EventBridge {
	return EventBridge{}
}

// WorkflowEventCmd returns a tea.Cmd that reads a single WorkflowEvent from
// ch and converts it to a WorkflowEventMsg. The command sends nil when the
// channel is closed or ctx is done.
//
// Usage: call repeatedly inside App.Update to keep draining the channel:
//
//	case WorkflowEventMsg:
//	    // handle...
//	    return a, bridge.WorkflowEventCmd(ctx, ch)
func (b EventBridge) WorkflowEventCmd(ctx context.Context, ch <-chan workflow.WorkflowEvent) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			return WorkflowEventMsg{
				WorkflowID:   ev.WorkflowID,
				WorkflowName: ev.WorkflowID, // WorkflowEvent has no separate Name field; use ID
				Step:         ev.Step,
				Event:        ev.Event,
				Detail:       ev.Message,
				Timestamp:    ev.Timestamp,
			}
		}
	}
}

// LoopEventCmd returns a tea.Cmd that reads a single LoopEvent from ch and
// converts it to the appropriate TUI message. If the event is a rate-limit
// event it emits a RateLimitMsg; otherwise it emits a LoopEventMsg. The
// command sends nil when the channel is closed or ctx is done.
//
// Usage: call repeatedly inside App.Update after receiving a LoopEventMsg:
//
//	case LoopEventMsg:
//	    // handle...
//	    return a, bridge.LoopEventCmd(ctx, ch)
func (b EventBridge) LoopEventCmd(ctx context.Context, ch <-chan loop.LoopEvent) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			return convertLoopEvent(ev)
		}
	}
}

// convertLoopEvent maps a loop.LoopEvent to a TUI message.
//
// Rate-limit events (EventRateLimitWait) are converted to RateLimitMsg so the
// sidebar countdown timer can start immediately. All other events are converted
// to LoopEventMsg using the mapLoopEventType helper.
func convertLoopEvent(ev loop.LoopEvent) tea.Msg {
	if ev.Type == loop.EventRateLimitWait {
		return RateLimitMsg{
			Agent:      ev.AgentName,
			Provider:   ev.AgentName, // provider == agent name when not specified separately
			ResetAfter: ev.WaitTime,
			ResetAt:    ev.Timestamp.Add(ev.WaitTime),
			Timestamp:  ev.Timestamp,
		}
	}

	return LoopEventMsg{
		Type:      mapLoopEventType(ev.Type),
		TaskID:    ev.TaskID,
		Iteration: ev.Iteration,
		Detail:    ev.Message,
		Timestamp: ev.Timestamp,
	}
}

// mapLoopEventType converts a loop.LoopEventType (string) to the TUI
// LoopEventType (int iota). Unmapped types default to LoopIterationStarted.
func mapLoopEventType(t loop.LoopEventType) LoopEventType {
	switch t {
	case loop.EventTaskSelected:
		return LoopTaskSelected
	case loop.EventTaskCompleted:
		return LoopTaskCompleted
	case loop.EventTaskBlocked:
		return LoopTaskBlocked
	case loop.EventRateLimitResume:
		return LoopResumedAfterWait
	case loop.EventPhaseComplete:
		return LoopPhaseComplete
	case loop.EventLoopError, loop.EventLoopAborted:
		return LoopError
	case loop.EventAgentStarted, loop.EventLoopStarted:
		return LoopIterationStarted
	case loop.EventAgentCompleted:
		return LoopIterationCompleted
	default:
		return LoopIterationStarted
	}
}

// AgentOutputCmd returns a tea.Cmd that reads a single AgentOutputMsg from
// ch and forwards it unchanged. The command sends nil when the channel is
// closed or ctx is done.
//
// Because AgentOutputMsg is already a TUI message type, no conversion is
// needed. This helper exists for symmetry with the other bridge methods.
func (b EventBridge) AgentOutputCmd(ctx context.Context, ch <-chan AgentOutputMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			return msg
		}
	}
}

// TaskProgressCmd returns a tea.Cmd that reads a single TaskProgressMsg from
// ch and forwards it unchanged. The command sends nil when the channel is
// closed or ctx is done.
func (b EventBridge) TaskProgressCmd(ctx context.Context, ch <-chan TaskProgressMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			return msg
		}
	}
}

// SendWorkflowEvent is a convenience function that sends a WorkflowEvent to
// the Bubble Tea program p by converting it to a WorkflowEventMsg. It is
// intended for use outside the Elm update loop (e.g., from a goroutine that
// monitors the workflow engine) when direct channel bridging is not used.
func SendWorkflowEvent(p *tea.Program, ev workflow.WorkflowEvent) {
	p.Send(WorkflowEventMsg{
		WorkflowID:   ev.WorkflowID,
		WorkflowName: ev.WorkflowID,
		Step:         ev.Step,
		Event:        ev.Event,
		Detail:       ev.Message,
		Timestamp:    ev.Timestamp,
	})
}

// SendLoopEvent is a convenience function that converts a loop.LoopEvent and
// sends the resulting TUI message to the Bubble Tea program p. It is intended
// for use from a monitoring goroutine when direct channel bridging is not used.
func SendLoopEvent(p *tea.Program, ev loop.LoopEvent) {
	p.Send(convertLoopEvent(ev))
}

// SendAgentOutput is a convenience function that sends an AgentOutputMsg to
// the Bubble Tea program p with the given agent name, output line, stream
// label, and timestamp.
func SendAgentOutput(p *tea.Program, agent, line, stream string, ts time.Time) {
	p.Send(AgentOutputMsg{
		Agent:     agent,
		Line:      line,
		Stream:    stream,
		Timestamp: ts,
	})
}
