package workflow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Transition event constants
// ---------------------------------------------------------------------------

func TestTransitionEventConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"EventSuccess", EventSuccess, "success"},
		{"EventFailure", EventFailure, "failure"},
		{"EventBlocked", EventBlocked, "blocked"},
		{"EventRateLimited", EventRateLimited, "rate_limited"},
		{"EventNeedsHuman", EventNeedsHuman, "needs_human"},
		{"EventPartial", EventPartial, "partial"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.got)
		})
	}
}

func TestTransitionEventConstants_Unique(t *testing.T) {
	t.Parallel()

	all := []string{
		EventSuccess, EventFailure, EventBlocked,
		EventRateLimited, EventNeedsHuman, EventPartial,
	}

	seen := make(map[string]struct{}, len(all))
	for _, v := range all {
		_, duplicate := seen[v]
		assert.False(t, duplicate, "duplicate transition event constant: %q", v)
		seen[v] = struct{}{}
	}
}

func TestTransitionEventConstants_NonEmpty(t *testing.T) {
	t.Parallel()

	all := []string{
		EventSuccess, EventFailure, EventBlocked,
		EventRateLimited, EventNeedsHuman, EventPartial,
	}
	for _, v := range all {
		assert.NotEmpty(t, v)
	}
}

// ---------------------------------------------------------------------------
// Terminal pseudo-step constants
// ---------------------------------------------------------------------------

func TestTerminalPseudoStepConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "__done__", StepDone)
	assert.Equal(t, "__failed__", StepFailed)
}

func TestTerminalPseudoSteps_DistinctFromEventConstants(t *testing.T) {
	t.Parallel()

	transitionEvents := []string{
		EventSuccess, EventFailure, EventBlocked,
		EventRateLimited, EventNeedsHuman, EventPartial,
	}
	pseudoSteps := []string{StepDone, StepFailed}

	for _, ps := range pseudoSteps {
		for _, ev := range transitionEvents {
			assert.NotEqual(t, ev, ps,
				"pseudo-step %q must not collide with transition event %q", ps, ev)
		}
	}
}

func TestTerminalPseudoSteps_PrefixConvention(t *testing.T) {
	t.Parallel()

	// Both terminal pseudo-steps must use the __ prefix so they cannot be
	// confused with ordinary user-defined step names.
	assert.Contains(t, StepDone, "__")
	assert.Contains(t, StepFailed, "__")
}

// ---------------------------------------------------------------------------
// WorkflowEventType constants
// ---------------------------------------------------------------------------

func TestWorkflowEventTypeConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"WEStepStarted", WEStepStarted, "step_started"},
		{"WEStepCompleted", WEStepCompleted, "step_completed"},
		{"WEStepFailed", WEStepFailed, "step_failed"},
		{"WEWorkflowStarted", WEWorkflowStarted, "workflow_started"},
		{"WEWorkflowCompleted", WEWorkflowCompleted, "workflow_completed"},
		{"WEWorkflowFailed", WEWorkflowFailed, "workflow_failed"},
		{"WEWorkflowResumed", WEWorkflowResumed, "workflow_resumed"},
		{"WEStepSkipped", WEStepSkipped, "step_skipped"},
		{"WECheckpoint", WECheckpoint, "checkpoint"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.got)
		})
	}
}

func TestWorkflowEventTypeConstants_Unique(t *testing.T) {
	t.Parallel()

	all := []string{
		WEStepStarted, WEStepCompleted, WEStepFailed,
		WEWorkflowStarted, WEWorkflowCompleted, WEWorkflowFailed,
		WEWorkflowResumed, WEStepSkipped, WECheckpoint,
	}

	seen := make(map[string]struct{}, len(all))
	for _, v := range all {
		_, duplicate := seen[v]
		assert.False(t, duplicate, "duplicate WorkflowEventType constant: %q", v)
		seen[v] = struct{}{}
	}
}

func TestWorkflowEventTypeConstants_NonEmpty(t *testing.T) {
	t.Parallel()

	all := []string{
		WEStepStarted, WEStepCompleted, WEStepFailed,
		WEWorkflowStarted, WEWorkflowCompleted, WEWorkflowFailed,
		WEWorkflowResumed, WEStepSkipped, WECheckpoint,
	}
	for _, v := range all {
		assert.NotEmpty(t, v)
	}
}

// ---------------------------------------------------------------------------
// WorkflowEvent JSON serialization
// ---------------------------------------------------------------------------

func TestWorkflowEvent_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)
	original := WorkflowEvent{
		Type:       WEStepCompleted,
		WorkflowID: "wf-abc",
		Step:       "implement",
		Event:      EventSuccess,
		Message:    "step completed successfully",
		Timestamp:  now,
		Error:      "",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded WorkflowEvent
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original.Type, decoded.Type)
	assert.Equal(t, original.WorkflowID, decoded.WorkflowID)
	assert.Equal(t, original.Step, decoded.Step)
	assert.Equal(t, original.Event, decoded.Event)
	assert.Equal(t, original.Message, decoded.Message)
	assert.Equal(t, original.Timestamp.UnixMilli(), decoded.Timestamp.UnixMilli())
	assert.Empty(t, decoded.Error)
}

func TestWorkflowEvent_JSONRoundTrip_WithError(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)
	original := WorkflowEvent{
		Type:       WEStepFailed,
		WorkflowID: "wf-xyz",
		Step:       "review",
		Event:      EventFailure,
		Message:    "step failed",
		Timestamp:  now,
		Error:      "connection refused",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded WorkflowEvent
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "connection refused", decoded.Error)
}

func TestWorkflowEvent_JSONStructTags(t *testing.T) {
	t.Parallel()

	ev := WorkflowEvent{
		Type:       WEWorkflowStarted,
		WorkflowID: "wf-tags",
		Step:       "init",
		Event:      EventSuccess,
		Message:    "started",
		Timestamp:  time.Now(),
	}

	data, err := json.Marshal(ev)
	require.NoError(t, err)
	raw := string(data)

	assert.Contains(t, raw, `"type"`)
	assert.Contains(t, raw, `"workflow_id"`)
	assert.Contains(t, raw, `"step"`)
	assert.Contains(t, raw, `"event"`)
	assert.Contains(t, raw, `"message"`)
	assert.Contains(t, raw, `"timestamp"`)
	// Error must be omitted when empty.
	assert.NotContains(t, raw, `"error"`)
}

func TestWorkflowEvent_ErrorOmitEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		errField   string
		wantInJSON bool
	}{
		{
			name:       "error omitted when empty",
			errField:   "",
			wantInJSON: false,
		},
		{
			name:       "error present when non-empty",
			errField:   "something went wrong",
			wantInJSON: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ev := WorkflowEvent{
				Type:      WEStepFailed,
				Timestamp: time.Now(),
				Error:     tt.errField,
			}
			data, err := json.Marshal(ev)
			require.NoError(t, err)
			if tt.wantInJSON {
				assert.Contains(t, string(data), `"error"`)
			} else {
				assert.NotContains(t, string(data), `"error"`)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// WorkflowDefinition JSON serialization
// ---------------------------------------------------------------------------

func TestWorkflowDefinition_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := WorkflowDefinition{
		Name:        "implement-pipeline",
		Description: "Full implementation workflow",
		InitialStep: "plan",
		Steps: []StepDefinition{
			{
				Name: "plan",
				Transitions: map[string]string{
					EventSuccess: "implement",
					EventFailure: StepFailed,
				},
			},
			{
				Name: "implement",
				Transitions: map[string]string{
					EventSuccess: StepDone,
					EventFailure: StepFailed,
					EventBlocked: "plan",
				},
				Parallel: true,
			},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded WorkflowDefinition
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Description, decoded.Description)
	assert.Equal(t, original.InitialStep, decoded.InitialStep)
	require.Len(t, decoded.Steps, 2)
	assert.Equal(t, "plan", decoded.Steps[0].Name)
	assert.Equal(t, "implement", decoded.Steps[1].Name)
	assert.True(t, decoded.Steps[1].Parallel)
}

func TestWorkflowDefinition_JSONStructTags(t *testing.T) {
	t.Parallel()

	wd := WorkflowDefinition{
		Name:        "test-wf",
		Description: "test",
		InitialStep: "start",
		Steps:       []StepDefinition{{Name: "start", Transitions: map[string]string{}}},
	}

	data, err := json.Marshal(wd)
	require.NoError(t, err)
	raw := string(data)

	assert.Contains(t, raw, `"name"`)
	assert.Contains(t, raw, `"description"`)
	assert.Contains(t, raw, `"steps"`)
	assert.Contains(t, raw, `"initial_step"`)
}

// ---------------------------------------------------------------------------
// StepDefinition JSON serialization
// ---------------------------------------------------------------------------

func TestStepDefinition_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := StepDefinition{
		Name: "review",
		Transitions: map[string]string{
			EventSuccess:     "fix",
			EventFailure:     StepFailed,
			EventRateLimited: "review",
		},
		Parallel: false,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded StepDefinition
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Transitions, decoded.Transitions)
	assert.False(t, decoded.Parallel)
}

func TestStepDefinition_EmptyTransitions(t *testing.T) {
	t.Parallel()

	// A terminal step has no outgoing transitions.
	terminal := StepDefinition{
		Name:        "terminal",
		Transitions: map[string]string{},
	}

	data, err := json.Marshal(terminal)
	require.NoError(t, err)

	var decoded StepDefinition
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Empty(t, decoded.Transitions)
}

func TestStepDefinition_NilTransitions(t *testing.T) {
	t.Parallel()

	// Nil transitions map should marshal to null and unmarshal back without error.
	sd := StepDefinition{Name: "no-transitions"}
	data, err := json.Marshal(sd)
	require.NoError(t, err)

	var decoded StepDefinition
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "no-transitions", decoded.Name)
}

func TestStepDefinition_JSONStructTags(t *testing.T) {
	t.Parallel()

	sd := StepDefinition{
		Name:        "step-a",
		Transitions: map[string]string{EventSuccess: "step-b"},
		Parallel:    true,
	}

	data, err := json.Marshal(sd)
	require.NoError(t, err)
	raw := string(data)

	assert.Contains(t, raw, `"name"`)
	assert.Contains(t, raw, `"transitions"`)
	assert.Contains(t, raw, `"parallel"`)
}

func TestStepDefinition_ParallelOmitEmpty(t *testing.T) {
	t.Parallel()

	// When Parallel is false it should be omitted from JSON due to omitempty.
	sd := StepDefinition{
		Name:        "serial-step",
		Transitions: map[string]string{},
		Parallel:    false,
	}

	data, err := json.Marshal(sd)
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"parallel"`)
}

// ---------------------------------------------------------------------------
// StepHandler interface compile-time check
// ---------------------------------------------------------------------------

// mockStepHandler is a minimal implementation used only to verify that the
// StepHandler interface can be satisfied by concrete types.
type mockStepHandler struct {
	name string
}

func (m *mockStepHandler) Execute(_ context.Context, _ *WorkflowState) (string, error) {
	return EventSuccess, nil
}

func (m *mockStepHandler) DryRun(_ *WorkflowState) string {
	return "dry-run: " + m.name
}

func (m *mockStepHandler) Name() string {
	return m.name
}

// Compile-time interface satisfaction check.
var _ StepHandler = (*mockStepHandler)(nil)

func TestStepHandler_Interface(t *testing.T) {
	t.Parallel()

	var h StepHandler = &mockStepHandler{name: "test-step"}
	assert.Equal(t, "test-step", h.Name())
	assert.Equal(t, "dry-run: test-step", h.DryRun(nil))
}
