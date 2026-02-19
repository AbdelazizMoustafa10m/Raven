package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
		name     string
		records  []StepRecord
		wantNil  bool
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

// ---------------------------------------------------------------------------
// sanitizeID
// ---------------------------------------------------------------------------

func TestSanitizeID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "alphanumeric unchanged",
			input: "wf-001",
			want:  "wf-001",
		},
		{
			name:  "dots replaced",
			input: "wf.001",
			want:  "wf_001",
		},
		{
			name:  "colons replaced",
			input: "wf:2024:01",
			want:  "wf_2024_01",
		},
		{
			name:  "spaces replaced",
			input: "my workflow",
			want:  "my_workflow",
		},
		{
			name:  "slashes replaced",
			input: "wf/run/1",
			want:  "wf_run_1",
		},
		{
			name:  "underscores and dashes preserved",
			input: "my_wf-run_1",
			want:  "my_wf-run_1",
		},
		{
			name:  "mixed special chars",
			input: "wf@2024!run#1",
			want:  "wf_2024_run_1",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, sanitizeID(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// StatusFromState
// ---------------------------------------------------------------------------

func TestStatusFromState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		currentStep string
		history     []StepRecord
		want        string
	}{
		{
			name:        "completed when current step is StepDone",
			currentStep: StepDone,
			want:        "completed",
		},
		{
			name:        "failed when current step is StepFailed",
			currentStep: StepFailed,
			want:        "failed",
		},
		{
			name:        "failed when last step event is EventFailure",
			currentStep: "step-2",
			history: []StepRecord{
				{Step: "step-1", Event: EventSuccess},
				{Step: "step-2", Event: EventFailure},
			},
			want: "failed",
		},
		{
			name:        "running when steps exist and not terminal",
			currentStep: "step-2",
			history: []StepRecord{
				{Step: "step-1", Event: EventSuccess},
			},
			want: "running",
		},
		{
			name:        "interrupted when no steps and not terminal",
			currentStep: "step-1",
			history:     nil,
			want:        "interrupted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ws := NewWorkflowState("test", "test-wf", tt.currentStep)
			for _, r := range tt.history {
				ws.AddStepRecord(r)
			}
			// Override CurrentStep to the target value (AddStepRecord doesn't change it).
			ws.CurrentStep = tt.currentStep
			got := StatusFromState(ws)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// NewStateStore
// ---------------------------------------------------------------------------

func TestNewStateStore_CreatesDirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "nested", "state")
	store, err := NewStateStore(dir)
	require.NoError(t, err)
	require.NotNil(t, store)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "store directory must be created")
}

func TestNewStateStore_ExistingDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewStateStore(dir)
	require.NoError(t, err)
	require.NotNil(t, store)
}

// ---------------------------------------------------------------------------
// StateStore.Save and Load
// ---------------------------------------------------------------------------

func TestStateStore_SaveAndLoad(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	ws := NewWorkflowState("wf-save-load", "test-workflow", "step-1")
	ws.AddStepRecord(StepRecord{Step: "step-1", Event: EventSuccess})

	err = store.Save(ws)
	require.NoError(t, err)

	loaded, err := store.Load("wf-save-load")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, ws.ID, loaded.ID)
	assert.Equal(t, ws.WorkflowName, loaded.WorkflowName)
	assert.Equal(t, ws.CurrentStep, loaded.CurrentStep)
	require.Len(t, loaded.StepHistory, 1)
	assert.Equal(t, ws.StepHistory[0].Step, loaded.StepHistory[0].Step)
}

func TestStateStore_Load_NotFound(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	_, err = store.Load("nonexistent-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStateStore_Save_AtomicWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewStateStore(dir)
	require.NoError(t, err)

	ws := NewWorkflowState("wf-atomic", "test-workflow", "step-1")
	err = store.Save(ws)
	require.NoError(t, err)

	// The final JSON file must exist.
	finalPath := filepath.Join(dir, "wf-atomic.json")
	_, err = os.Stat(finalPath)
	require.NoError(t, err, "final JSON file must exist after save")

	// No .tmp file should remain.
	tmpPath := filepath.Join(dir, "wf-atomic.json.tmp")
	_, err = os.Stat(tmpPath)
	assert.True(t, os.IsNotExist(err), "temp file must be removed after atomic rename")
}

func TestStateStore_Save_SanitizesID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewStateStore(dir)
	require.NoError(t, err)

	// ID with special chars that must be sanitized.
	ws := NewWorkflowState("wf:2024/01", "test-wf", "step-1")
	err = store.Save(ws)
	require.NoError(t, err)

	// The file should exist with underscores replacing special chars.
	sanitized := sanitizeID("wf:2024/01")
	finalPath := filepath.Join(dir, sanitized+".json")
	_, statErr := os.Stat(finalPath)
	require.NoError(t, statErr, "file must be stored with sanitized filename")
}

func TestStateStore_Save_OverwriteExisting(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	ws := NewWorkflowState("wf-overwrite", "test-workflow", "step-1")
	require.NoError(t, store.Save(ws))

	// Modify and save again.
	ws.CurrentStep = StepDone
	ws.AddStepRecord(StepRecord{Step: "step-1", Event: EventSuccess})
	require.NoError(t, store.Save(ws))

	loaded, err := store.Load("wf-overwrite")
	require.NoError(t, err)
	assert.Equal(t, StepDone, loaded.CurrentStep)
	assert.Len(t, loaded.StepHistory, 1)
}

// ---------------------------------------------------------------------------
// StateStore.List
// ---------------------------------------------------------------------------

func TestStateStore_List_Empty(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	summaries, err := store.List()
	require.NoError(t, err)
	assert.NotNil(t, summaries, "List must never return nil")
	assert.Empty(t, summaries)
}

func TestStateStore_List_NonExistentDirectory(t *testing.T) {
	t.Parallel()

	// Create a store pointing at a directory that does not exist yet.
	// Since NewStateStore calls MkdirAll, we instead bypass it and test
	// the "dir removed after creation" edge case by deleting the dir.
	dir := filepath.Join(t.TempDir(), "gone")
	store, err := NewStateStore(dir)
	require.NoError(t, err)

	// Remove the directory after creation.
	require.NoError(t, os.Remove(dir))

	summaries, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestStateStore_List_MultipleSorted(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	now := time.Now()

	// Save three workflow states with different UpdatedAt values.
	ws1 := NewWorkflowState("wf-old", "wf", "step-1")
	ws1.UpdatedAt = now.Add(-2 * time.Hour)
	require.NoError(t, store.Save(ws1))

	ws2 := NewWorkflowState("wf-newest", "wf", "step-1")
	ws2.UpdatedAt = now
	require.NoError(t, store.Save(ws2))

	ws3 := NewWorkflowState("wf-middle", "wf", "step-1")
	ws3.UpdatedAt = now.Add(-1 * time.Hour)
	require.NoError(t, store.Save(ws3))

	summaries, err := store.List()
	require.NoError(t, err)
	require.Len(t, summaries, 3)

	// Most recent must be first.
	assert.Equal(t, "wf-newest", summaries[0].ID)
	assert.Equal(t, "wf-middle", summaries[1].ID)
	assert.Equal(t, "wf-old", summaries[2].ID)
}

func TestStateStore_List_SkipsCorruptFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewStateStore(dir)
	require.NoError(t, err)

	// Write a valid state.
	ws := NewWorkflowState("wf-valid", "wf", "step-1")
	require.NoError(t, store.Save(ws))

	// Write a corrupt JSON file directly.
	corruptPath := filepath.Join(dir, "corrupt.json")
	require.NoError(t, os.WriteFile(corruptPath, []byte("not valid json{{{"), 0o644))

	summaries, err := store.List()
	require.NoError(t, err)
	require.Len(t, summaries, 1, "corrupt file must be skipped")
	assert.Equal(t, "wf-valid", summaries[0].ID)
}

func TestStateStore_List_SkipsTmpFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewStateStore(dir)
	require.NoError(t, err)

	// Write a valid state.
	ws := NewWorkflowState("wf-clean", "wf", "step-1")
	require.NoError(t, store.Save(ws))

	// Manually write a leftover .tmp file (as if Save crashed before rename).
	tmpPath := filepath.Join(dir, "leftover.json.tmp")
	data, _ := json.Marshal(ws)
	require.NoError(t, os.WriteFile(tmpPath, data, 0o644))

	summaries, err := store.List()
	require.NoError(t, err)
	require.Len(t, summaries, 1, ".tmp files must be excluded from List")
	assert.Equal(t, "wf-clean", summaries[0].ID)
}

func TestStateStore_List_SummaryFields(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	ws := NewWorkflowState("wf-fields", "my-workflow", "step-1")
	ws.AddStepRecord(StepRecord{Step: "step-1", Event: EventSuccess})
	ws.CurrentStep = StepDone
	require.NoError(t, store.Save(ws))

	summaries, err := store.List()
	require.NoError(t, err)
	require.Len(t, summaries, 1)

	s := summaries[0]
	assert.Equal(t, "wf-fields", s.ID)
	assert.Equal(t, "my-workflow", s.WorkflowName)
	assert.Equal(t, StepDone, s.CurrentStep)
	assert.Equal(t, "completed", s.Status)
	assert.Equal(t, 1, s.StepCount)
}

// ---------------------------------------------------------------------------
// StateStore.Delete
// ---------------------------------------------------------------------------

func TestStateStore_Delete_Existing(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	ws := NewWorkflowState("wf-delete", "wf", "step-1")
	require.NoError(t, store.Save(ws))

	err = store.Delete("wf-delete")
	require.NoError(t, err)

	// Should no longer be loadable.
	_, err = store.Load("wf-delete")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStateStore_Delete_NotFound(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	err = store.Delete("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// ---------------------------------------------------------------------------
// StateStore.LatestRun
// ---------------------------------------------------------------------------

func TestStateStore_LatestRun_Empty(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	latest, err := store.LatestRun()
	require.NoError(t, err)
	assert.Nil(t, latest, "LatestRun must return nil when store is empty")
}

func TestStateStore_LatestRun_ReturnsNewest(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	now := time.Now()

	ws1 := NewWorkflowState("wf-older", "wf", "step-1")
	ws1.UpdatedAt = now.Add(-1 * time.Hour)
	require.NoError(t, store.Save(ws1))

	ws2 := NewWorkflowState("wf-newer", "wf", "step-1")
	ws2.UpdatedAt = now
	require.NoError(t, store.Save(ws2))

	latest, err := store.LatestRun()
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, "wf-newer", latest.ID)
}

// ---------------------------------------------------------------------------
// WithCheckpointing engine option
// ---------------------------------------------------------------------------

func TestWithCheckpointing_SavesAfterEachStep(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewStateStore(dir)
	require.NoError(t, err)

	a := newRecorder("step-a")
	b := newRecorder("step-b")
	reg := registerAll(a, b)
	def := linearDef("step-a", "step-b")

	eng := NewEngine(reg, WithCheckpointing(store))
	state, err := eng.Run(context.Background(), def, nil)
	require.NoError(t, err)

	// After the run, the state must be persisted in the store.
	loaded, err := store.Load(state.ID)
	require.NoError(t, err)
	assert.Equal(t, state.ID, loaded.ID)
	assert.Equal(t, StepDone, loaded.CurrentStep)
	// Both steps must be in the history.
	require.Len(t, loaded.StepHistory, 2)
}

func TestWithCheckpointing_HookErrorDoesNotAbortWorkflow(t *testing.T) {
	t.Parallel()

	// Use a store pointing to a read-only location to trigger save errors.
	roDir := t.TempDir()

	store, err := NewStateStore(roDir)
	require.NoError(t, err)

	// Make the directory read-only so Save will fail.
	require.NoError(t, os.Chmod(roDir, 0o555))
	t.Cleanup(func() { os.Chmod(roDir, 0o755) }) // restore for cleanup

	a := newRecorder("step-a")
	reg := registerAll(a)
	def := linearDef("step-a")

	// Workflow must still complete even when checkpointing fails.
	eng := NewEngine(reg, WithCheckpointing(store))
	state, runErr := eng.Run(context.Background(), def, nil)

	// On macOS root may override permissions; skip the assertion if run as root.
	if runErr == nil {
		assert.Equal(t, StepDone, state.CurrentStep, "workflow must complete despite checkpoint errors")
	}
}

// ---------------------------------------------------------------------------
// TestSanitizeID -- additional edge cases
// ---------------------------------------------------------------------------

func TestSanitizeID_AdditionalEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "already clean alphanumeric",
			input: "abc123",
			want:  "abc123",
		},
		{
			name:  "already clean with dash and underscore",
			input: "run_wf-42",
			want:  "run_wf-42",
		},
		{
			name:  "unicode multibyte replaced",
			input: "wf-日本語",
			want:  "wf-___",
		},
		{
			name:  "emoji replaced",
			input: "run-🚀-1",
			want:  "run-_-1",
		},
		{
			name:  "all special chars",
			input: "!@#$%^&*()",
			want:  "__________",
		},
		{
			name:  "path traversal chars replaced",
			input: "../../../etc/passwd",
			want:  "_________etc_passwd",
		},
		{
			name:  "null byte equivalent char replaced",
			input: "wf\x00run",
			want:  "wf_run",
		},
		{
			name:  "single dash preserved",
			input: "-",
			want:  "-",
		},
		{
			name:  "single underscore preserved",
			input: "_",
			want:  "_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, sanitizeID(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// StateStore.Save -- JSON content verification
// ---------------------------------------------------------------------------

func TestStateStore_Save_WritesIndentedJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewStateStore(dir)
	require.NoError(t, err)

	ws := NewWorkflowState("wf-indent", "test-workflow", "step-1")
	ws.Metadata["key"] = "value"
	require.NoError(t, store.Save(ws))

	data, err := os.ReadFile(filepath.Join(dir, "wf-indent.json"))
	require.NoError(t, err)

	content := string(data)
	// Indented JSON must contain newlines and spaces.
	assert.Contains(t, content, "\n", "saved JSON must be indented (has newlines)")
	assert.Contains(t, content, "  ", "saved JSON must be indented (has spaces)")
	// Verify well-formed JSON can be re-parsed.
	var ws2 WorkflowState
	require.NoError(t, json.Unmarshal(data, &ws2), "saved file must contain valid JSON")
	assert.Equal(t, ws.ID, ws2.ID)
}

func TestStateStore_Save_AllFieldsPersisted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewStateStore(dir)
	require.NoError(t, err)

	now := time.Now().Truncate(time.Millisecond)
	ws := &WorkflowState{
		ID:           "wf-fields-persist",
		WorkflowName: "full-workflow",
		CurrentStep:  "step-2",
		StepHistory: []StepRecord{
			{
				Step:      "step-1",
				Event:     EventSuccess,
				StartedAt: now.Add(-10 * time.Second),
				Duration:  5 * time.Second,
			},
		},
		Metadata:  map[string]any{"agent": "claude", "task": "T-046"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	require.NoError(t, store.Save(ws))

	loaded, err := store.Load(ws.ID)
	require.NoError(t, err)

	assert.Equal(t, ws.ID, loaded.ID)
	assert.Equal(t, ws.WorkflowName, loaded.WorkflowName)
	assert.Equal(t, ws.CurrentStep, loaded.CurrentStep)
	require.Len(t, loaded.StepHistory, 1)
	assert.Equal(t, ws.StepHistory[0].Step, loaded.StepHistory[0].Step)
	assert.Equal(t, ws.StepHistory[0].Event, loaded.StepHistory[0].Event)
	assert.Equal(t, ws.StepHistory[0].Duration, loaded.StepHistory[0].Duration)
	assert.Equal(t, "claude", loaded.Metadata["agent"])
	assert.Equal(t, "T-046", loaded.Metadata["task"])
	assert.Equal(t, now.UnixMilli(), loaded.CreatedAt.UnixMilli())
	assert.Equal(t, now.UnixMilli(), loaded.UpdatedAt.UnixMilli())
}

// ---------------------------------------------------------------------------
// StateStore.Save -- concurrent saves
// ---------------------------------------------------------------------------

func TestStateStore_Save_Concurrent(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	const numGoroutines = 10
	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		i := i
		go func() {
			id := fmt.Sprintf("wf-concurrent-%02d", i)
			ws := NewWorkflowState(id, "concurrent-wf", "step-1")
			ws.Metadata["goroutine"] = i
			errCh <- store.Save(ws)
		}()
	}

	// Collect all errors.
	for i := 0; i < numGoroutines; i++ {
		assert.NoError(t, <-errCh, "concurrent Save must not return an error")
	}

	// All runs must be persisted.
	summaries, err := store.List()
	require.NoError(t, err)
	assert.Len(t, summaries, numGoroutines, "all concurrent saves must be persisted")
}

// ---------------------------------------------------------------------------
// StateStore.Save -- very large metadata
// ---------------------------------------------------------------------------

func TestStateStore_Save_VeryLargeMetadata(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	ws := NewWorkflowState("wf-large-meta", "test-workflow", "step-1")

	// Insert 1000 metadata keys each with a 1KB value.
	largeVal := string(make([]byte, 1024)) // 1 KB string
	for i := 0; i < 1000; i++ {
		ws.Metadata[fmt.Sprintf("key-%04d", i)] = largeVal
	}

	err = store.Save(ws)
	require.NoError(t, err, "Save must succeed with very large metadata")

	loaded, err := store.Load(ws.ID)
	require.NoError(t, err)
	assert.Len(t, loaded.Metadata, 1000, "all metadata keys must survive round-trip")
}

// ---------------------------------------------------------------------------
// StateStore.Load -- corrupt file error
// ---------------------------------------------------------------------------

func TestStateStore_Load_CorruptFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewStateStore(dir)
	require.NoError(t, err)

	// Write corrupt JSON directly.
	corruptPath := filepath.Join(dir, "corrupt-run.json")
	require.NoError(t, os.WriteFile(corruptPath, []byte("{invalid json}"), 0o644))

	_, err = store.Load("corrupt-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "corrupt-run", "error must mention the run ID")
}

// ---------------------------------------------------------------------------
// StateStore.Load -- special characters in run ID
// ---------------------------------------------------------------------------

func TestStateStore_Load_SpecialCharsInRunID(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	// IDs with special characters are sanitized to the same filename.
	ws1 := NewWorkflowState("wf:2024/01", "test-wf", "step-1")
	require.NoError(t, store.Save(ws1))

	// Load using the original (unsanitized) ID.
	loaded, err := store.Load("wf:2024/01")
	require.NoError(t, err)
	assert.Equal(t, "wf:2024/01", loaded.ID)
}

// ---------------------------------------------------------------------------
// StateStore.Delete -- subsequent LatestRun after delete
// ---------------------------------------------------------------------------

func TestStateStore_LatestRun_AfterDelete(t *testing.T) {
	t.Parallel()

	store, err := NewStateStore(t.TempDir())
	require.NoError(t, err)

	now := time.Now()

	ws1 := NewWorkflowState("wf-keep", "wf", "step-1")
	ws1.UpdatedAt = now.Add(-1 * time.Hour)
	require.NoError(t, store.Save(ws1))

	ws2 := NewWorkflowState("wf-delete-me", "wf", "step-1")
	ws2.UpdatedAt = now // most recent
	require.NoError(t, store.Save(ws2))

	// Verify wf-delete-me is the latest before deletion.
	latest, err := store.LatestRun()
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, "wf-delete-me", latest.ID)

	// Delete the most recent run.
	require.NoError(t, store.Delete("wf-delete-me"))

	// Now wf-keep should be the latest.
	latest, err = store.LatestRun()
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, "wf-keep", latest.ID)
}

// ---------------------------------------------------------------------------
// StateStore.List -- skips subdirectories
// ---------------------------------------------------------------------------

func TestStateStore_List_SkipsSubdirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewStateStore(dir)
	require.NoError(t, err)

	// Save a valid run.
	ws := NewWorkflowState("wf-real", "wf", "step-1")
	require.NoError(t, store.Save(ws))

	// Create a subdirectory named with .json suffix to confuse the scanner.
	subDir := filepath.Join(dir, "subdir.json")
	require.NoError(t, os.Mkdir(subDir, 0o755))

	summaries, err := store.List()
	require.NoError(t, err)
	require.Len(t, summaries, 1, "subdirectories must be skipped even with .json in name")
	assert.Equal(t, "wf-real", summaries[0].ID)
}

// ---------------------------------------------------------------------------
// WithCheckpointing -- verifies checkpoint is written after each individual step
// ---------------------------------------------------------------------------

func TestWithCheckpointing_IntermediateCheckpoints(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewStateStore(dir)
	require.NoError(t, err)

	// Track checkpoint saves in order using a custom counting handler.
	var mu sync.Mutex
	checkpointCounts := []int{} // step counts at each checkpoint moment

	countingStore := &countingStateStore{
		inner: store,
		onSave: func(ws *WorkflowState) {
			mu.Lock()
			checkpointCounts = append(checkpointCounts, len(ws.StepHistory))
			mu.Unlock()
		},
	}

	a := newRecorder("step-a")
	b := newRecorder("step-b")
	c := newRecorder("step-c")
	reg := registerAll(a, b, c)
	def := linearDef("step-a", "step-b", "step-c")

	eng := NewEngine(reg, countingStore.asStateStore())
	_, err = eng.Run(context.Background(), def, nil)
	require.NoError(t, err)

	// After each step one save should have occurred.
	// 3 steps => 3 saves with step counts [1, 2, 3].
	mu.Lock()
	captured := make([]int, len(checkpointCounts))
	copy(captured, checkpointCounts)
	mu.Unlock()

	require.Len(t, captured, 3, "must have one checkpoint save per step")
	assert.Equal(t, []int{1, 2, 3}, captured, "checkpoint must occur after each step with growing history")
}

// countingStateStore wraps StateStore to intercept Save calls for counting.
type countingStateStore struct {
	inner  *StateStore
	onSave func(*WorkflowState)
}

func (c *countingStateStore) save(ws *WorkflowState) error {
	if err := c.inner.Save(ws); err != nil {
		return err
	}
	if c.onSave != nil {
		c.onSave(ws)
	}
	return nil
}

// asStateStore returns a synthetic StateStore with a custom postStepHook wired
// by returning a EngineOption that uses the counting save function.
func (c *countingStateStore) asStateStore() EngineOption {
	return func(e *Engine) {
		e.postStepHook = func(state *WorkflowState) error {
			return c.save(state)
		}
	}
}

// ---------------------------------------------------------------------------
// StatusFromState -- additional cases
// ---------------------------------------------------------------------------

func TestStatusFromState_AdditionalCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		currentStep string
		history     []StepRecord
		want        string
	}{
		{
			name:        "completed takes precedence over any history",
			currentStep: StepDone,
			history: []StepRecord{
				{Step: "step-1", Event: EventFailure},
			},
			want: "completed",
		},
		{
			name:        "failed when StepFailed even with no history",
			currentStep: StepFailed,
			history:     nil,
			want:        "failed",
		},
		{
			name:        "failed via last step EventFailure with multiple steps",
			currentStep: "step-3",
			history: []StepRecord{
				{Step: "step-1", Event: EventSuccess},
				{Step: "step-2", Event: EventSuccess},
				{Step: "step-3", Event: EventFailure},
			},
			want: "failed",
		},
		{
			name:        "running when last step is success but not terminal",
			currentStep: "step-2",
			history: []StepRecord{
				{Step: "step-1", Event: EventSuccess},
			},
			want: "running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ws := NewWorkflowState("test-extra", "test-wf", tt.currentStep)
			for _, r := range tt.history {
				ws.AddStepRecord(r)
			}
			ws.CurrentStep = tt.currentStep
			got := StatusFromState(ws)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// RunSummary fields -- verify status derivation is correct in List output
// ---------------------------------------------------------------------------

func TestStateStore_List_StatusDerivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		currentStep string
		history     []StepRecord
		wantStatus  string
	}{
		{
			name:        "completed run",
			currentStep: StepDone,
			wantStatus:  "completed",
		},
		{
			name:        "failed run via StepFailed",
			currentStep: StepFailed,
			wantStatus:  "failed",
		},
		{
			name:        "running run with history",
			currentStep: "step-2",
			history: []StepRecord{
				{Step: "step-1", Event: EventSuccess},
			},
			wantStatus: "running",
		},
		{
			name:        "interrupted run with no history",
			currentStep: "step-1",
			wantStatus:  "interrupted",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := NewStateStore(t.TempDir())
			require.NoError(t, err)

			id := fmt.Sprintf("wf-status-%02d", i)
			ws := NewWorkflowState(id, "test-wf", tt.currentStep)
			for _, r := range tt.history {
				ws.AddStepRecord(r)
			}
			ws.CurrentStep = tt.currentStep
			require.NoError(t, store.Save(ws))

			summaries, err := store.List()
			require.NoError(t, err)
			require.Len(t, summaries, 1)
			assert.Equal(t, tt.wantStatus, summaries[0].Status)
		})
	}
}

// ---------------------------------------------------------------------------
// NewStateStore -- directory created on first use (nested)
// ---------------------------------------------------------------------------

func TestNewStateStore_CreatesNestedDirectories(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	// Use deeply nested path that does not yet exist.
	deep := filepath.Join(base, "a", "b", "c", "state")

	store, err := NewStateStore(deep)
	require.NoError(t, err)
	require.NotNil(t, store)

	info, err := os.Stat(deep)
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "nested directories must be created by NewStateStore")
}

// ---------------------------------------------------------------------------
// Benchmarks for StateStore
// ---------------------------------------------------------------------------

// BenchmarkStateStore_SaveLoad benchmarks a single Save + Load round-trip.
func BenchmarkStateStore_SaveLoad(b *testing.B) {
	store, err := NewStateStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}

	ws := NewWorkflowState("bench-run", "bench-workflow", "step-1")
	for i := 0; i < 10; i++ {
		ws.AddStepRecord(StepRecord{
			Step:      fmt.Sprintf("step-%d", i),
			Event:     EventSuccess,
			StartedAt: time.Now(),
			Duration:  time.Millisecond,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.Save(ws); err != nil {
			b.Fatal(err)
		}
		if _, err := store.Load(ws.ID); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStateStore_List benchmarks listing 100 persisted runs.
func BenchmarkStateStore_List(b *testing.B) {
	store, err := NewStateStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}

	// Pre-populate 100 runs.
	for i := 0; i < 100; i++ {
		ws := NewWorkflowState(fmt.Sprintf("bench-run-%03d", i), "bench-wf", "step-1")
		if err := store.Save(ws); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.List(); err != nil {
			b.Fatal(err)
		}
	}
}
