package task

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTask_IsReady(t *testing.T) {
	tests := []struct {
		name      string
		deps      []string
		completed map[string]bool
		want      bool
	}{
		{
			name:      "no dependencies is always ready",
			deps:      nil,
			completed: map[string]bool{},
			want:      true,
		},
		{
			name:      "empty dependencies is always ready",
			deps:      []string{},
			completed: map[string]bool{},
			want:      true,
		},
		{
			name:      "all dependencies completed",
			deps:      []string{"T-001", "T-002"},
			completed: map[string]bool{"T-001": true, "T-002": true},
			want:      true,
		},
		{
			name:      "some dependencies not completed",
			deps:      []string{"T-001", "T-002"},
			completed: map[string]bool{"T-001": true},
			want:      false,
		},
		{
			name:      "no dependencies completed",
			deps:      []string{"T-001", "T-002"},
			completed: map[string]bool{},
			want:      false,
		},
		{
			name:      "nil completed map with dependencies",
			deps:      []string{"T-001"},
			completed: nil,
			want:      false,
		},
		{
			name:      "nil completed map without dependencies",
			deps:      nil,
			completed: nil,
			want:      true,
		},
		{
			name:      "dependency explicitly set to false",
			deps:      []string{"T-001"},
			completed: map[string]bool{"T-001": false},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			task := &Task{
				ID:           "T-999",
				Dependencies: tt.deps,
			}
			assert.Equal(t, tt.want, task.IsReady(tt.completed))
		})
	}
}

func TestValidStatus(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		want   bool
	}{
		{name: "not_started is valid", status: StatusNotStarted, want: true},
		{name: "in_progress is valid", status: StatusInProgress, want: true},
		{name: "completed is valid", status: StatusCompleted, want: true},
		{name: "blocked is valid", status: StatusBlocked, want: true},
		{name: "skipped is valid", status: StatusSkipped, want: true},
		{name: "empty string is invalid", status: "", want: false},
		{name: "unknown status is invalid", status: "unknown", want: false},
		{name: "COMPLETED uppercase is invalid", status: "COMPLETED", want: false},
		{name: "random string is invalid", status: "foobar", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ValidStatus(tt.status))
		})
	}
}

func TestTaskStatus_StringValues(t *testing.T) {
	t.Parallel()

	// Verify that TaskStatus constants are string values, not iota.
	assert.Equal(t, TaskStatus("not_started"), StatusNotStarted)
	assert.Equal(t, TaskStatus("in_progress"), StatusInProgress)
	assert.Equal(t, TaskStatus("completed"), StatusCompleted)
	assert.Equal(t, TaskStatus("blocked"), StatusBlocked)
	assert.Equal(t, TaskStatus("skipped"), StatusSkipped)
}

func TestTask_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	task := Task{
		ID:           "T-004",
		Title:        "Central Data Types",
		Status:       StatusInProgress,
		Phase:        1,
		Dependencies: []string{"T-001", "T-002", "T-003"},
		SpecFile:     "docs/tasks/T-004-central-types.md",
	}

	data, err := json.Marshal(task)
	require.NoError(t, err)

	var decoded Task
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, task, decoded)
}

func TestTask_JSONStructTags(t *testing.T) {
	t.Parallel()

	task := Task{
		ID:           "T-001",
		Title:        "Test",
		Status:       StatusNotStarted,
		Phase:        1,
		Dependencies: []string{"T-000"},
		SpecFile:     "test.md",
	}

	data, err := json.Marshal(task)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"id"`)
	assert.Contains(t, raw, `"title"`)
	assert.Contains(t, raw, `"status"`)
	assert.Contains(t, raw, `"phase"`)
	assert.Contains(t, raw, `"dependencies"`)
	assert.Contains(t, raw, `"spec_file"`)
}

func TestTask_StatusSerializesAsString(t *testing.T) {
	t.Parallel()

	task := Task{
		ID:     "T-001",
		Status: StatusCompleted,
	}

	data, err := json.Marshal(task)
	require.NoError(t, err)

	assert.Contains(t, string(data), `"status":"completed"`)
}

func TestTask_EmptyDependenciesSerializesAsArray(t *testing.T) {
	t.Parallel()

	task := Task{
		ID:           "T-001",
		Dependencies: []string{},
	}

	data, err := json.Marshal(task)
	require.NoError(t, err)

	assert.Contains(t, string(data), `"dependencies":[]`)
}

func TestTask_NilDependenciesSerializesAsNull(t *testing.T) {
	t.Parallel()

	// This test documents the behavior: nil slices serialize as null.
	// Constructors should initialize to empty slices to avoid this.
	task := Task{
		ID:           "T-001",
		Dependencies: nil,
	}

	data, err := json.Marshal(task)
	require.NoError(t, err)

	assert.Contains(t, string(data), `"dependencies":null`)
}

func TestPhase_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	phase := Phase{
		ID:        1,
		Name:      "Foundation",
		StartTask: "T-001",
		EndTask:   "T-015",
	}

	data, err := json.Marshal(phase)
	require.NoError(t, err)

	var decoded Phase
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, phase, decoded)
}

func TestPhase_JSONStructTags(t *testing.T) {
	t.Parallel()

	phase := Phase{
		ID:        2,
		Name:      "Task System",
		StartTask: "T-016",
		EndTask:   "T-030",
	}

	data, err := json.Marshal(phase)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"id"`)
	assert.Contains(t, raw, `"name"`)
	assert.Contains(t, raw, `"start_task"`)
	assert.Contains(t, raw, `"end_task"`)
}
