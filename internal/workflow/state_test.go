package workflow

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWorkflowState(t *testing.T) {
	t.Parallel()

	ws := NewWorkflowState("wf-001", "implement", "init")

	assert.Equal(t, "wf-001", ws.ID)
	assert.Equal(t, "implement", ws.WorkflowName)
	assert.Equal(t, "init", ws.CurrentStep)
	assert.NotNil(t, ws.StepHistory, "StepHistory must not be nil")
	assert.Empty(t, ws.StepHistory, "StepHistory must be empty")
	assert.NotNil(t, ws.Metadata, "Metadata must not be nil")
	assert.Empty(t, ws.Metadata, "Metadata must be empty")
	assert.False(t, ws.CreatedAt.IsZero(), "CreatedAt must be set")
	assert.False(t, ws.UpdatedAt.IsZero(), "UpdatedAt must be set")
	assert.Equal(t, ws.CreatedAt, ws.UpdatedAt, "CreatedAt and UpdatedAt should match at creation")
}

func TestNewWorkflowState_EmptySliceJSON(t *testing.T) {
	t.Parallel()

	ws := NewWorkflowState("wf-002", "review", "start")
	data, err := json.Marshal(ws)
	require.NoError(t, err)

	// Verify that step_history serializes as [] not null
	assert.Contains(t, string(data), `"step_history":[]`)
	// Verify that metadata serializes as {} not null
	assert.Contains(t, string(data), `"metadata":{}`)
}

func TestAddStepRecord(t *testing.T) {
	t.Parallel()

	ws := NewWorkflowState("wf-003", "pipeline", "step-1")
	originalUpdatedAt := ws.UpdatedAt

	// Small delay to ensure UpdatedAt changes.
	time.Sleep(time.Millisecond)

	record := StepRecord{
		Step:      "step-1",
		Event:     "completed",
		StartedAt: time.Now().Add(-5 * time.Second),
		Duration:  5 * time.Second,
	}
	ws.AddStepRecord(record)

	require.Len(t, ws.StepHistory, 1)
	assert.Equal(t, "step-1", ws.StepHistory[0].Step)
	assert.Equal(t, "completed", ws.StepHistory[0].Event)
	assert.Equal(t, 5*time.Second, ws.StepHistory[0].Duration)
	assert.True(t, ws.UpdatedAt.After(originalUpdatedAt), "UpdatedAt must be updated after AddStepRecord")
}

func TestAddStepRecord_Multiple(t *testing.T) {
	t.Parallel()

	ws := NewWorkflowState("wf-004", "pipeline", "step-1")

	ws.AddStepRecord(StepRecord{Step: "step-1", Event: "done"})
	ws.AddStepRecord(StepRecord{Step: "step-2", Event: "done"})
	ws.AddStepRecord(StepRecord{Step: "step-3", Event: "failed", Error: "timeout"})

	require.Len(t, ws.StepHistory, 3)
	assert.Equal(t, "step-3", ws.StepHistory[2].Step)
	assert.Equal(t, "timeout", ws.StepHistory[2].Error)
}

func TestLastStep(t *testing.T) {
	tests := []struct {
		name    string
		records []StepRecord
		wantNil bool
		wantStep string
	}{
		{
			name:    "no steps returns nil",
			records: nil,
			wantNil: true,
		},
		{
			name: "single step",
			records: []StepRecord{
				{Step: "init", Event: "completed"},
			},
			wantStep: "init",
		},
		{
			name: "multiple steps returns last",
			records: []StepRecord{
				{Step: "init", Event: "completed"},
				{Step: "build", Event: "completed"},
				{Step: "deploy", Event: "failed"},
			},
			wantStep: "deploy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ws := NewWorkflowState("test", "test-wf", "start")
			for _, r := range tt.records {
				ws.AddStepRecord(r)
			}

			got := ws.LastStep()
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.wantStep, got.Step)
		})
	}
}

func TestWorkflowState_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)
	ws := &WorkflowState{
		ID:           "wf-rt",
		WorkflowName: "roundtrip",
		CurrentStep:  "step-2",
		StepHistory: []StepRecord{
			{
				Step:      "step-1",
				Event:     "completed",
				StartedAt: now.Add(-10 * time.Second),
				Duration:  3 * time.Second,
			},
		},
		Metadata: map[string]any{
			"agent": "claude",
			"count": float64(42),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(ws)
	require.NoError(t, err)

	var decoded WorkflowState
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, ws.ID, decoded.ID)
	assert.Equal(t, ws.WorkflowName, decoded.WorkflowName)
	assert.Equal(t, ws.CurrentStep, decoded.CurrentStep)
	require.Len(t, decoded.StepHistory, 1)
	assert.Equal(t, ws.StepHistory[0].Step, decoded.StepHistory[0].Step)
	assert.Equal(t, ws.StepHistory[0].Duration, decoded.StepHistory[0].Duration)
	assert.Equal(t, "claude", decoded.Metadata["agent"])
	// JSON numbers deserialize as float64.
	assert.Equal(t, float64(42), decoded.Metadata["count"])
}

func TestWorkflowState_JSONStructTags(t *testing.T) {
	t.Parallel()

	ws := NewWorkflowState("tag-test", "tags", "init")
	data, err := json.Marshal(ws)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"id"`)
	assert.Contains(t, raw, `"workflow_name"`)
	assert.Contains(t, raw, `"current_step"`)
	assert.Contains(t, raw, `"step_history"`)
	assert.Contains(t, raw, `"metadata"`)
	assert.Contains(t, raw, `"created_at"`)
	assert.Contains(t, raw, `"updated_at"`)
}

func TestStepRecord_JSONStructTags(t *testing.T) {
	t.Parallel()

	record := StepRecord{
		Step:      "test-step",
		Event:     "completed",
		StartedAt: time.Now(),
		Duration:  2 * time.Second,
	}

	data, err := json.Marshal(record)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"step"`)
	assert.Contains(t, raw, `"event"`)
	assert.Contains(t, raw, `"started_at"`)
	assert.Contains(t, raw, `"duration"`)
	// Error should be omitted when empty.
	assert.NotContains(t, raw, `"error"`)
}

func TestStepRecord_ErrorOmitEmpty(t *testing.T) {
	t.Parallel()

	// With error set, it should appear in JSON.
	record := StepRecord{
		Step:  "fail",
		Event: "error",
		Error: "something broke",
	}
	data, err := json.Marshal(record)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"error":"something broke"`)

	// Without error, it should be omitted.
	record2 := StepRecord{
		Step:  "ok",
		Event: "completed",
	}
	data2, err := json.Marshal(record2)
	require.NoError(t, err)
	assert.NotContains(t, string(data2), `"error"`)
}

func TestDuration_SerializesAsNanoseconds(t *testing.T) {
	t.Parallel()

	record := StepRecord{
		Step:     "timed",
		Event:    "done",
		Duration: 5 * time.Second,
	}

	data, err := json.Marshal(record)
	require.NoError(t, err)

	var raw map[string]any
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	// 5 seconds = 5,000,000,000 nanoseconds.
	assert.Equal(t, float64(5*time.Second), raw["duration"])
}
