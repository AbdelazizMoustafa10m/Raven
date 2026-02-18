package task

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- parseLine tests --------------------------------------------------------

func TestParseLine_FullLine(t *testing.T) {
	t.Parallel()

	line := "T-001|completed|claude|2026-02-17T10:30:00Z|Foundation setup done"
	state, err := parseLine(line)
	require.NoError(t, err)

	assert.Equal(t, "T-001", state.TaskID)
	assert.Equal(t, StatusCompleted, state.Status)
	assert.Equal(t, "claude", state.Agent)
	assert.Equal(t, "Foundation setup done", state.Notes)

	want, _ := time.Parse(time.RFC3339, "2026-02-17T10:30:00Z")
	assert.Equal(t, want, state.Timestamp)
}

func TestParseLine_MinimalLine(t *testing.T) {
	t.Parallel()

	// Only task ID present with four pipe separators; all other fields empty.
	line := "T-004||||"
	state, err := parseLine(line)
	require.NoError(t, err)

	assert.Equal(t, "T-004", state.TaskID)
	assert.Equal(t, TaskStatus(""), state.Status)
	assert.Equal(t, "", state.Agent)
	assert.True(t, state.Timestamp.IsZero())
	assert.Equal(t, "", state.Notes)
}

func TestParseLine_OnlyTaskID(t *testing.T) {
	t.Parallel()

	// Only task ID, no pipe separators at all -- minimal valid input.
	line := "T-004"
	state, err := parseLine(line)
	require.NoError(t, err)

	assert.Equal(t, "T-004", state.TaskID)
	assert.Equal(t, TaskStatus(""), state.Status)
	assert.Equal(t, "", state.Agent)
	assert.True(t, state.Timestamp.IsZero())
	assert.Equal(t, "", state.Notes)
}

func TestParseLine_NotStartedFormat(t *testing.T) {
	t.Parallel()

	// Matches the canonical not_started format from the spec.
	line := "T-004|not_started|||"
	state, err := parseLine(line)
	require.NoError(t, err)

	assert.Equal(t, "T-004", state.TaskID)
	assert.Equal(t, StatusNotStarted, state.Status)
	assert.Equal(t, "", state.Agent)
	assert.True(t, state.Timestamp.IsZero())
	assert.Equal(t, "", state.Notes)
}

func TestParseLine_NotesContainPipe(t *testing.T) {
	t.Parallel()

	// The notes field may contain pipe characters; split is limited to 5.
	line := "T-005|completed|claude|2026-02-17T10:30:00Z|note with | pipe inside"
	state, err := parseLine(line)
	require.NoError(t, err)

	assert.Equal(t, "T-005", state.TaskID)
	assert.Equal(t, "note with | pipe inside", state.Notes)
}

func TestParseLine_NotesContainMultiplePipes(t *testing.T) {
	t.Parallel()

	// Notes may contain many pipe characters; only the first 4 field boundaries
	// are split.
	line := "T-007|skipped|codex|2026-02-17T10:30:00Z|a|b|c|d"
	state, err := parseLine(line)
	require.NoError(t, err)

	assert.Equal(t, "T-007", state.TaskID)
	assert.Equal(t, "a|b|c|d", state.Notes, "all content after the 5th split is the notes field")
}

func TestParseLine_EmptyTaskID(t *testing.T) {
	t.Parallel()

	_, err := parseLine("|completed|claude||")
	assert.Error(t, err)
}

func TestParseLine_WhitespaceOnlyTaskID(t *testing.T) {
	t.Parallel()

	// Task ID with only whitespace should be rejected.
	_, err := parseLine("   |completed|claude||")
	assert.Error(t, err)
}

func TestParseLine_BadTimestamp(t *testing.T) {
	t.Parallel()

	// Non-RFC3339 timestamp: best-effort, zero time expected.
	line := "T-001|in_progress|codex|not-a-timestamp|some note"
	state, err := parseLine(line)
	require.NoError(t, err)
	assert.True(t, state.Timestamp.IsZero(), "expected zero time for unparseable timestamp")
}

func TestParseLine_VariousNonRFC3339Timestamps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		timestamp string
	}{
		{name: "date only", timestamp: "2026-02-17"},
		{name: "unix epoch string", timestamp: "1700000000"},
		{name: "human readable", timestamp: "Feb 17 2026"},
		{name: "empty string", timestamp: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			line := fmt.Sprintf("T-001|in_progress|claude|%s|some note", tt.timestamp)
			state, err := parseLine(line)
			require.NoError(t, err)
			assert.True(t, state.Timestamp.IsZero(),
				"expected zero time for non-RFC3339 timestamp %q", tt.timestamp)
		})
	}
}

func TestParseLine_TaskIDPreservedAsIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		taskID string
	}{
		{name: "short padding T-01", taskID: "T-01"},
		{name: "standard padding T-001", taskID: "T-001"},
		{name: "long padding T-0001", taskID: "T-0001"},
		{name: "no padding T-1", taskID: "T-1"},
		{name: "alphanumeric T-01A", taskID: "T-01A"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			line := fmt.Sprintf("%s|not_started|||", tt.taskID)
			state, err := parseLine(line)
			require.NoError(t, err)
			assert.Equal(t, tt.taskID, state.TaskID,
				"task ID must be preserved exactly as written, without normalization")
		})
	}
}

func TestParseLine_AllStatusValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusStr  string
		wantStatus TaskStatus
	}{
		{name: "not_started", statusStr: "not_started", wantStatus: StatusNotStarted},
		{name: "in_progress", statusStr: "in_progress", wantStatus: StatusInProgress},
		{name: "completed", statusStr: "completed", wantStatus: StatusCompleted},
		{name: "blocked", statusStr: "blocked", wantStatus: StatusBlocked},
		{name: "skipped", statusStr: "skipped", wantStatus: StatusSkipped},
		{name: "unknown preserved", statusStr: "custom_status", wantStatus: TaskStatus("custom_status")},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			line := fmt.Sprintf("T-001|%s|||", tt.statusStr)
			state, err := parseLine(line)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, state.Status)
		})
	}
}

// ---- formatLine tests -------------------------------------------------------

func TestFormatLine_Full(t *testing.T) {
	t.Parallel()

	ts, _ := time.Parse(time.RFC3339, "2026-02-17T10:30:00Z")
	state := TaskState{
		TaskID:    "T-001",
		Status:    StatusCompleted,
		Agent:     "claude",
		Timestamp: ts,
		Notes:     "Foundation setup done",
	}
	got := formatLine(state)
	assert.Equal(t, "T-001|completed|claude|2026-02-17T10:30:00Z|Foundation setup done", got)
}

func TestFormatLine_EmptyOptionals(t *testing.T) {
	t.Parallel()

	state := TaskState{
		TaskID: "T-004",
		Status: StatusNotStarted,
	}
	got := formatLine(state)
	assert.Equal(t, "T-004|not_started|||", got)
}

func TestFormatLine_ZeroTimestamp(t *testing.T) {
	t.Parallel()

	state := TaskState{
		TaskID: "T-010",
		Status: StatusBlocked,
	}
	got := formatLine(state)
	// Timestamp portion must be empty (no zero-value RFC3339 string).
	assert.Equal(t, "T-010|blocked|||", got)
}

func TestFormatLine_TimestampAlwaysUTC(t *testing.T) {
	t.Parallel()

	// Timestamp with a non-UTC fixed offset must be normalized to UTC in output.
	// Use a fixed-offset zone (UTC-5) so the test does not depend on the
	// system timezone database.
	est := time.FixedZone("EST", -5*60*60)
	ts := time.Date(2026, 2, 17, 5, 30, 0, 0, est) // 05:30 EST = 10:30 UTC

	state := TaskState{
		TaskID:    "T-001",
		Status:    StatusCompleted,
		Agent:     "claude",
		Timestamp: ts,
	}
	got := formatLine(state)
	// Output must contain the UTC representation.
	assert.Contains(t, got, "2026-02-17T10:30:00Z")
}

func TestFormatLine_NotesWithPipePassthrough(t *testing.T) {
	t.Parallel()

	state := TaskState{
		TaskID: "T-005",
		Status: StatusCompleted,
		Agent:  "claude",
		Notes:  "note with | pipe | inside",
	}
	got := formatLine(state)
	assert.Contains(t, got, "note with | pipe | inside")
}

func TestFormatLine_AllStatusConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status TaskStatus
		want   string
	}{
		{StatusNotStarted, "not_started"},
		{StatusInProgress, "in_progress"},
		{StatusCompleted, "completed"},
		{StatusBlocked, "blocked"},
		{StatusSkipped, "skipped"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()
			state := TaskState{TaskID: "T-001", Status: tt.status}
			got := formatLine(state)
			assert.Contains(t, got, tt.want)
		})
	}
}

// ---- Round-trip tests -------------------------------------------------------

func TestRoundTrip_ParseThenFormat(t *testing.T) {
	t.Parallel()

	original := "T-003|in_progress|codex|2026-02-17T12:00:00Z|Working on config"
	state, err := parseLine(original)
	require.NoError(t, err)

	got := formatLine(*state)
	assert.Equal(t, original, got)
}

func TestRoundTrip_FormatThenParse(t *testing.T) {
	t.Parallel()

	// Construct a TaskState, format it, then parse it back.
	// The resulting state must be identical to the original.
	ts := time.Date(2026, 2, 18, 14, 0, 0, 0, time.UTC)
	original := TaskState{
		TaskID:    "T-007",
		Status:    StatusInProgress,
		Agent:     "codex",
		Timestamp: ts,
		Notes:     "Implementation underway",
	}

	line := formatLine(original)
	parsed, err := parseLine(line)
	require.NoError(t, err)

	assert.Equal(t, original.TaskID, parsed.TaskID)
	assert.Equal(t, original.Status, parsed.Status)
	assert.Equal(t, original.Agent, parsed.Agent)
	assert.Equal(t, original.Notes, parsed.Notes)
	assert.True(t, original.Timestamp.Equal(parsed.Timestamp),
		"timestamps must be equal: got %v, want %v", parsed.Timestamp, original.Timestamp)
}

func TestRoundTrip_FormatThenParse_AllStatuses(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 2, 17, 10, 30, 0, 0, time.UTC)
	for _, status := range ValidStatuses() {
		status := status
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			original := TaskState{
				TaskID:    "T-001",
				Status:    status,
				Agent:     "claude",
				Timestamp: ts,
				Notes:     "some note",
			}
			line := formatLine(original)
			parsed, err := parseLine(line)
			require.NoError(t, err)
			assert.Equal(t, original.Status, parsed.Status)
		})
	}
}

func TestRoundTrip_EmptyTimestamp(t *testing.T) {
	t.Parallel()

	// A state with a zero timestamp should survive a round-trip.
	original := TaskState{
		TaskID: "T-008",
		Status: StatusNotStarted,
	}

	line := formatLine(original)
	parsed, err := parseLine(line)
	require.NoError(t, err)

	assert.Equal(t, original.TaskID, parsed.TaskID)
	assert.Equal(t, original.Status, parsed.Status)
	assert.True(t, parsed.Timestamp.IsZero(), "zero timestamp must survive round-trip as zero")
}

// ---- Load tests -------------------------------------------------------------

func TestLoad_ValidFixture(t *testing.T) {
	t.Parallel()

	sm := NewStateManager(fixtureStatePath(t, "valid.conf"))
	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, 5, "valid.conf should have 5 entries")
}

func TestLoad_ValidFixture_AllFieldsCorrect(t *testing.T) {
	t.Parallel()

	sm := NewStateManager(fixtureStatePath(t, "valid.conf"))
	states, err := sm.Load()
	require.NoError(t, err)
	require.Len(t, states, 5)

	// Verify each row from valid.conf is parsed correctly.
	tests := []struct {
		idx    int
		taskID string
		status TaskStatus
		agent  string
		notes  string
	}{
		{0, "T-001", StatusCompleted, "claude", "Foundation setup done"},
		{1, "T-002", StatusInProgress, "codex", "Working on config"},
		{2, "T-003", StatusNotStarted, "", ""},
		{3, "T-004", StatusBlocked, "claude", "Waiting on T-001"},
		{4, "T-005", StatusSkipped, "", "Decided to skip this task"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.taskID, states[tt.idx].TaskID, "row %d task ID", tt.idx)
		assert.Equal(t, tt.status, states[tt.idx].Status, "row %d status", tt.idx)
		assert.Equal(t, tt.agent, states[tt.idx].Agent, "row %d agent", tt.idx)
		assert.Equal(t, tt.notes, states[tt.idx].Notes, "row %d notes", tt.idx)
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	t.Parallel()

	sm := NewStateManager(fixtureStatePath(t, "empty.conf"))
	states, err := sm.Load()
	require.NoError(t, err)
	assert.Empty(t, states, "empty.conf should yield zero entries")
}

func TestLoad_NonExistentFile(t *testing.T) {
	t.Parallel()

	sm := NewStateManager("/tmp/raven-test-nonexistent-state.conf")
	states, err := sm.Load()
	require.NoError(t, err)
	assert.Empty(t, states)
}

func TestLoad_SkipsCommentsAndBlankLines(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "# comment\n\nT-001|completed|claude|2026-02-17T10:00:00Z|\n\n# another comment\n")
	sm := NewStateManager(path)
	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, 1)
}

func TestLoad_TrailingNewline_NoPhantomEntry(t *testing.T) {
	t.Parallel()

	// A file ending with \n must not produce a phantom empty entry.
	content := "T-001|completed|claude|2026-02-17T10:00:00Z|done\n"
	path := writeTempState(t, content)
	sm := NewStateManager(path)

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, 1, "trailing newline must not create a phantom entry")
	assert.Equal(t, "T-001", states[0].TaskID)
}

func TestLoad_MultipleTrailingNewlines_NoPhantomEntries(t *testing.T) {
	t.Parallel()

	content := "T-001|completed|claude|2026-02-17T10:00:00Z|done\n\n\n"
	path := writeTempState(t, content)
	sm := NewStateManager(path)

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, 1, "multiple trailing newlines must not create phantom entries")
}

func TestLoad_BlankLinesWithinFile(t *testing.T) {
	t.Parallel()

	content := "T-001|completed|claude|2026-02-17T10:00:00Z|\n\nT-002|not_started|||\n\n"
	path := writeTempState(t, content)
	sm := NewStateManager(path)

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, 2, "blank lines within file must be skipped gracefully")
}

func TestLoad_OrderPreserved(t *testing.T) {
	t.Parallel()

	content := "T-003|not_started|||\nT-001|completed|claude|2026-02-17T10:00:00Z|\nT-002|in_progress|codex||\n"
	path := writeTempState(t, content)
	sm := NewStateManager(path)

	states, err := sm.Load()
	require.NoError(t, err)
	require.Len(t, states, 3)

	// Insertion order must be preserved.
	assert.Equal(t, "T-003", states[0].TaskID)
	assert.Equal(t, "T-001", states[1].TaskID)
	assert.Equal(t, "T-002", states[2].TaskID)
}

// ---- LoadMap tests ----------------------------------------------------------

func TestLoadMap_KeyedByTaskID(t *testing.T) {
	t.Parallel()

	sm := NewStateManager(fixtureStatePath(t, "valid.conf"))
	m, err := sm.LoadMap()
	require.NoError(t, err)

	require.Contains(t, m, "T-001")
	assert.Equal(t, StatusCompleted, m["T-001"].Status)
}

func TestLoadMap_AllEntriesPresent(t *testing.T) {
	t.Parallel()

	sm := NewStateManager(fixtureStatePath(t, "valid.conf"))
	m, err := sm.LoadMap()
	require.NoError(t, err)

	expectedIDs := []string{"T-001", "T-002", "T-003", "T-004", "T-005"}
	for _, id := range expectedIDs {
		assert.Contains(t, m, id, "map must contain task %s", id)
	}
	assert.Len(t, m, 5)
}

func TestLoadMap_NonExistentFile(t *testing.T) {
	t.Parallel()

	sm := NewStateManager("/tmp/raven-test-nonexistent-map.conf")
	m, err := sm.LoadMap()
	require.NoError(t, err)
	assert.Empty(t, m)
}

func TestLoadMap_CorrectValues(t *testing.T) {
	t.Parallel()

	content := "T-001|completed|claude|2026-02-17T10:30:00Z|done\nT-002|in_progress|codex||\n"
	path := writeTempState(t, content)
	sm := NewStateManager(path)

	m, err := sm.LoadMap()
	require.NoError(t, err)

	require.Contains(t, m, "T-001")
	assert.Equal(t, StatusCompleted, m["T-001"].Status)
	assert.Equal(t, "claude", m["T-001"].Agent)
	assert.Equal(t, "done", m["T-001"].Notes)

	require.Contains(t, m, "T-002")
	assert.Equal(t, StatusInProgress, m["T-002"].Status)
}

// ---- Get tests --------------------------------------------------------------

func TestGet_ExistingTask(t *testing.T) {
	t.Parallel()

	sm := NewStateManager(fixtureStatePath(t, "valid.conf"))
	state, err := sm.Get("T-001")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "T-001", state.TaskID)
	assert.Equal(t, StatusCompleted, state.Status)
}

func TestGet_NonExistentTask(t *testing.T) {
	t.Parallel()

	sm := NewStateManager(fixtureStatePath(t, "valid.conf"))
	state, err := sm.Get("T-999")
	require.NoError(t, err)
	assert.Nil(t, state, "unknown task should return nil, not an error")
}

func TestGet_AllFixtureTasksRetrievable(t *testing.T) {
	t.Parallel()

	sm := NewStateManager(fixtureStatePath(t, "valid.conf"))

	ids := []string{"T-001", "T-002", "T-003", "T-004", "T-005"}
	for _, id := range ids {
		id := id
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			state, err := sm.Get(id)
			require.NoError(t, err)
			require.NotNil(t, state, "Get(%q) must not return nil", id)
			assert.Equal(t, id, state.TaskID)
		})
	}
}

func TestGet_NonExistentFile(t *testing.T) {
	t.Parallel()

	sm := NewStateManager("/tmp/raven-test-nonexistent-get.conf")
	state, err := sm.Get("T-001")
	require.NoError(t, err)
	assert.Nil(t, state, "non-existent file should return nil, nil")
}

// ---- Update tests -----------------------------------------------------------

func TestUpdate_AppendNewTask(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "T-001|completed|claude|2026-02-17T10:00:00Z|\n")
	sm := NewStateManager(path)

	newState := TaskState{
		TaskID:    "T-002",
		Status:    StatusInProgress,
		Agent:     "codex",
		Timestamp: mustParseRFC3339("2026-02-18T08:00:00Z"),
		Notes:     "working",
	}
	require.NoError(t, sm.Update(newState))

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, 2)
	assert.Equal(t, "T-002", states[1].TaskID)
}

func TestUpdate_ModifyExistingTask(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "T-001|in_progress|claude|2026-02-17T10:00:00Z|\n")
	sm := NewStateManager(path)

	updated := TaskState{
		TaskID:    "T-001",
		Status:    StatusCompleted,
		Agent:     "claude",
		Timestamp: mustParseRFC3339("2026-02-18T09:00:00Z"),
		Notes:     "done",
	}
	require.NoError(t, sm.Update(updated))

	state, err := sm.Get("T-001")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, StatusCompleted, state.Status)
	assert.Equal(t, "done", state.Notes)
}

func TestUpdate_ModifyExistingTask_OtherTasksUnchanged(t *testing.T) {
	t.Parallel()

	content := "T-001|not_started|||\nT-002|completed|claude|2026-02-17T10:00:00Z|original\nT-003|blocked|||\n"
	path := writeTempState(t, content)
	sm := NewStateManager(path)

	// Update only T-001.
	require.NoError(t, sm.Update(TaskState{
		TaskID: "T-001",
		Status: StatusInProgress,
		Agent:  "codex",
	}))

	// Verify T-002 and T-003 are untouched.
	t2, err := sm.Get("T-002")
	require.NoError(t, err)
	require.NotNil(t, t2)
	assert.Equal(t, StatusCompleted, t2.Status)
	assert.Equal(t, "original", t2.Notes)

	t3, err := sm.Get("T-003")
	require.NoError(t, err)
	require.NotNil(t, t3)
	assert.Equal(t, StatusBlocked, t3.Status)
}

func TestUpdate_EmptyTaskIDReturnsError(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "")
	sm := NewStateManager(path)

	err := sm.Update(TaskState{TaskID: "", Status: StatusCompleted})
	assert.Error(t, err)
}

func TestUpdate_CreatesParentDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "task-state.conf")
	sm := NewStateManager(path)

	err := sm.Update(TaskState{TaskID: "T-001", Status: StatusNotStarted})
	require.NoError(t, err)

	_, statErr := os.Stat(path)
	assert.NoError(t, statErr, "state file should have been created")
}

func TestUpdate_OnNonExistentFile_CreatesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	sm := NewStateManager(path)

	require.NoError(t, sm.Update(TaskState{TaskID: "T-001", Status: StatusNotStarted}))

	states, err := sm.Load()
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "T-001", states[0].TaskID)
}

func TestUpdate_PreservesNotes(t *testing.T) {
	t.Parallel()

	// Notes with pipe characters must survive an Update round-trip.
	original := "T-001|not_started|||notes with | pipe | here\n"
	path := writeTempState(t, original)
	sm := NewStateManager(path)

	// Fetch, mutate status only, re-save.
	state, err := sm.Get("T-001")
	require.NoError(t, err)
	require.NotNil(t, state)

	state.Status = StatusInProgress
	require.NoError(t, sm.Update(*state))

	updated, err := sm.Get("T-001")
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "notes with | pipe | here", updated.Notes)
}

func TestUpdate_LargeFile_UpdateExistingEntry(t *testing.T) {
	t.Parallel()

	const n = 100
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString(fmt.Sprintf("T-%03d|not_started|||\n", i))
	}
	path := writeTempState(t, sb.String())
	sm := NewStateManager(path)

	// Update the middle entry.
	require.NoError(t, sm.Update(TaskState{
		TaskID: "T-050",
		Status: StatusCompleted,
		Agent:  "claude",
		Notes:  "large file update",
	}))

	state, err := sm.Get("T-050")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, StatusCompleted, state.Status)
	assert.Equal(t, "large file update", state.Notes)

	// Total count must remain n.
	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, n)
}

// ---- UpdateStatus tests -----------------------------------------------------

func TestUpdateStatus_SetsFieldsPreservesNotes(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "T-001|not_started|||my existing note\n")
	sm := NewStateManager(path)

	require.NoError(t, sm.UpdateStatus("T-001", StatusInProgress, "claude"))

	state, err := sm.Get("T-001")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, StatusInProgress, state.Status)
	assert.Equal(t, "claude", state.Agent)
	assert.Equal(t, "my existing note", state.Notes)
	assert.False(t, state.Timestamp.IsZero())
}

func TestUpdateStatus_TimestampIsRecentAndUTC(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "")
	sm := NewStateManager(path)

	before := time.Now().UTC().Add(-time.Second)
	require.NoError(t, sm.UpdateStatus("T-001", StatusInProgress, "claude"))
	after := time.Now().UTC().Add(time.Second)

	state, err := sm.Get("T-001")
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, time.UTC, state.Timestamp.Location(),
		"timestamp must be UTC")
	assert.True(t, state.Timestamp.After(before),
		"timestamp must be after the time before the call")
	assert.True(t, state.Timestamp.Before(after),
		"timestamp must be before the time after the call")
}

func TestUpdateStatus_NewTask(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "")
	sm := NewStateManager(path)

	require.NoError(t, sm.UpdateStatus("T-010", StatusInProgress, "gemini"))

	state, err := sm.Get("T-010")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, StatusInProgress, state.Status)
	assert.Equal(t, "", state.Notes, "new task should have empty notes")
}

func TestUpdateStatus_EmptyTaskIDReturnsError(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "")
	sm := NewStateManager(path)

	err := sm.UpdateStatus("", StatusCompleted, "claude")
	assert.Error(t, err)
}

func TestUpdateStatus_MultipleUpdates_StatusChanges(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "T-001|not_started|||initial\n")
	sm := NewStateManager(path)

	transitions := []TaskStatus{StatusInProgress, StatusCompleted}
	for _, s := range transitions {
		require.NoError(t, sm.UpdateStatus("T-001", s, "claude"))
	}

	state, err := sm.Get("T-001")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, StatusCompleted, state.Status)
	// Notes must still be preserved from original.
	assert.Equal(t, "initial", state.Notes)

	// Total entries must remain 1.
	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, 1)
}

// ---- Initialize tests -------------------------------------------------------

func TestInitialize_CreatesEntriesForNewTasks(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "")
	sm := NewStateManager(path)

	ids := []string{"T-001", "T-002", "T-003"}
	require.NoError(t, sm.Initialize(ids))

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, 3)
	for _, s := range states {
		assert.Equal(t, StatusNotStarted, s.Status)
	}
}

func TestInitialize_DoesNotOverwriteExistingEntries(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "T-001|completed|claude|2026-02-17T10:00:00Z|\n")
	sm := NewStateManager(path)

	require.NoError(t, sm.Initialize([]string{"T-001", "T-002"}))

	m, err := sm.LoadMap()
	require.NoError(t, err)

	// T-001 must remain completed.
	require.Contains(t, m, "T-001")
	assert.Equal(t, StatusCompleted, m["T-001"].Status)

	// T-002 must be added as not_started.
	require.Contains(t, m, "T-002")
	assert.Equal(t, StatusNotStarted, m["T-002"].Status)
}

func TestInitialize_SkipsEmptyIDs(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "")
	sm := NewStateManager(path)

	require.NoError(t, sm.Initialize([]string{"T-001", "", "T-002"}))

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, 2)
}

func TestInitialize_DeduplicatesIDs(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "")
	sm := NewStateManager(path)

	require.NoError(t, sm.Initialize([]string{"T-001", "T-001", "T-002"}))

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, 2)
}

func TestInitialize_EmptyIDList(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "")
	sm := NewStateManager(path)

	require.NoError(t, sm.Initialize([]string{}))

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Empty(t, states)
}

func TestInitialize_NilIDList(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "")
	sm := NewStateManager(path)

	require.NoError(t, sm.Initialize(nil))

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Empty(t, states)
}

func TestInitialize_PreservesExistingNotes(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "T-001|in_progress|claude||preserved note\n")
	sm := NewStateManager(path)

	require.NoError(t, sm.Initialize([]string{"T-001", "T-002"}))

	state, err := sm.Get("T-001")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, StatusInProgress, state.Status,
		"existing status must not be overwritten by Initialize")
	assert.Equal(t, "preserved note", state.Notes,
		"existing notes must be preserved")
}

func TestInitialize_OnNonExistentFile_CreatesEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	sm := NewStateManager(path)

	require.NoError(t, sm.Initialize([]string{"T-001", "T-002"}))

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, 2)
	for _, s := range states {
		assert.Equal(t, StatusNotStarted, s.Status)
	}
}

// ---- StatusCounts tests -----------------------------------------------------

func TestStatusCounts_MixedStatuses(t *testing.T) {
	t.Parallel()

	sm := NewStateManager(fixtureStatePath(t, "valid.conf"))
	counts, err := sm.StatusCounts()
	require.NoError(t, err)

	// valid.conf has one entry per status: completed, in_progress, not_started,
	// blocked, skipped.
	assert.Equal(t, 1, counts[StatusCompleted])
	assert.Equal(t, 1, counts[StatusInProgress])
	assert.Equal(t, 1, counts[StatusNotStarted])
	assert.Equal(t, 1, counts[StatusBlocked])
	assert.Equal(t, 1, counts[StatusSkipped])
}

func TestStatusCounts_EmptyFile(t *testing.T) {
	t.Parallel()

	sm := NewStateManager(fixtureStatePath(t, "empty.conf"))
	counts, err := sm.StatusCounts()
	require.NoError(t, err)
	assert.Empty(t, counts)
}

func TestStatusCounts_NonExistentFile(t *testing.T) {
	t.Parallel()

	sm := NewStateManager("/tmp/raven-test-nonexistent-counts.conf")
	counts, err := sm.StatusCounts()
	require.NoError(t, err)
	assert.Empty(t, counts)
}

func TestStatusCounts_SingleStatus(t *testing.T) {
	t.Parallel()

	content := "T-001|not_started|||\nT-002|not_started|||\nT-003|not_started|||\n"
	path := writeTempState(t, content)
	sm := NewStateManager(path)

	counts, err := sm.StatusCounts()
	require.NoError(t, err)
	assert.Equal(t, 3, counts[StatusNotStarted])
	assert.Equal(t, 0, counts[StatusCompleted])
}

func TestStatusCounts_SumEqualsTotal(t *testing.T) {
	t.Parallel()

	// The sum of all status counts must equal the total number of tasks.
	sm := NewStateManager(fixtureStatePath(t, "valid.conf"))

	states, err := sm.Load()
	require.NoError(t, err)

	counts, err := sm.StatusCounts()
	require.NoError(t, err)

	total := 0
	for _, c := range counts {
		total += c
	}
	assert.Equal(t, len(states), total, "sum of status counts must equal total task count")
}

// ---- TasksWithStatus tests --------------------------------------------------

func TestTasksWithStatus_Completed(t *testing.T) {
	t.Parallel()

	sm := NewStateManager(fixtureStatePath(t, "valid.conf"))
	ids, err := sm.TasksWithStatus(StatusCompleted)
	require.NoError(t, err)
	assert.Equal(t, []string{"T-001"}, ids)
}

func TestTasksWithStatus_NoMatch(t *testing.T) {
	t.Parallel()

	path := writeTempState(t, "T-001|not_started|||\n")
	sm := NewStateManager(path)

	ids, err := sm.TasksWithStatus(StatusCompleted)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestTasksWithStatus_MultipleMatches(t *testing.T) {
	t.Parallel()

	content := "T-001|completed|claude|2026-02-17T10:00:00Z|\nT-002|not_started|||\nT-003|completed|codex|2026-02-17T11:00:00Z|\nT-004|in_progress|claude||\n"
	path := writeTempState(t, content)
	sm := NewStateManager(path)

	ids, err := sm.TasksWithStatus(StatusCompleted)
	require.NoError(t, err)
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, "T-001")
	assert.Contains(t, ids, "T-003")
}

func TestTasksWithStatus_OrderPreserved(t *testing.T) {
	t.Parallel()

	// The returned IDs must preserve file order.
	content := "T-003|not_started|||\nT-001|not_started|||\nT-002|not_started|||\n"
	path := writeTempState(t, content)
	sm := NewStateManager(path)

	ids, err := sm.TasksWithStatus(StatusNotStarted)
	require.NoError(t, err)
	assert.Equal(t, []string{"T-003", "T-001", "T-002"}, ids)
}

func TestTasksWithStatus_AllStatuses(t *testing.T) {
	t.Parallel()

	sm := NewStateManager(fixtureStatePath(t, "valid.conf"))

	tests := []struct {
		status TaskStatus
		count  int
	}{
		{StatusCompleted, 1},
		{StatusInProgress, 1},
		{StatusNotStarted, 1},
		{StatusBlocked, 1},
		{StatusSkipped, 1},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()
			ids, err := sm.TasksWithStatus(tt.status)
			require.NoError(t, err)
			assert.Len(t, ids, tt.count)
		})
	}
}

func TestTasksWithStatus_NonExistentFile(t *testing.T) {
	t.Parallel()

	sm := NewStateManager("/tmp/raven-test-nonexistent-filter.conf")
	ids, err := sm.TasksWithStatus(StatusCompleted)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

// ---- IsValid / ValidStatuses tests ------------------------------------------

func TestIsValid_AllKnownStatuses(t *testing.T) {
	t.Parallel()

	for _, s := range ValidStatuses() {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			assert.True(t, s.IsValid(), "expected %q to be valid", s)
		})
	}
}

func TestIsValid_UnknownStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status TaskStatus
	}{
		{name: "unknown_status", status: TaskStatus("unknown_status")},
		{name: "empty string", status: TaskStatus("")},
		{name: "uppercase COMPLETED", status: TaskStatus("COMPLETED")},
		{name: "partial in_prog", status: TaskStatus("in_prog")},
		{name: "with spaces", status: TaskStatus("not started")},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.False(t, tt.status.IsValid(), "expected %q to be invalid", tt.status)
		})
	}
}

func TestValidStatuses_ContainsAllFive(t *testing.T) {
	t.Parallel()

	statuses := ValidStatuses()
	assert.Len(t, statuses, 5)
	assert.Contains(t, statuses, StatusNotStarted)
	assert.Contains(t, statuses, StatusInProgress)
	assert.Contains(t, statuses, StatusCompleted)
	assert.Contains(t, statuses, StatusBlocked)
	assert.Contains(t, statuses, StatusSkipped)
}

func TestValidStatuses_NoDuplicates(t *testing.T) {
	t.Parallel()

	statuses := ValidStatuses()
	seen := make(map[TaskStatus]bool, len(statuses))
	for _, s := range statuses {
		assert.False(t, seen[s], "duplicate status %q in ValidStatuses()", s)
		seen[s] = true
	}
}

// ---- Concurrent update integration test -------------------------------------

func TestUpdate_ConcurrentUpdates(t *testing.T) {
	// Multiple goroutines update different tasks simultaneously; the final file
	// must contain all entries without corruption.
	path := writeTempState(t, "")
	sm := NewStateManager(path)

	var wg sync.WaitGroup
	tasks := []string{"T-010", "T-011", "T-012", "T-013", "T-014"}

	for _, id := range tasks {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := sm.UpdateStatus(id, StatusInProgress, "claude")
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, len(tasks), "all concurrent updates should be persisted")
}

func TestUpdateStatus_ConcurrentWritesDifferentTasks(t *testing.T) {
	// 20 goroutines each write a unique task; no corruption must occur.
	const goroutines = 20
	path := writeTempState(t, "")
	sm := NewStateManager(path)

	var wg sync.WaitGroup
	for i := 1; i <= goroutines; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			taskID := fmt.Sprintf("T-%03d", i)
			err := sm.UpdateStatus(taskID, StatusInProgress, "claude")
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, goroutines,
		"all %d concurrent updates must be persisted without corruption", goroutines)

	// Verify all task IDs are present (no duplicates, no missing).
	ids := make(map[string]bool, goroutines)
	for _, s := range states {
		ids[s.TaskID] = true
	}
	for i := 1; i <= goroutines; i++ {
		assert.True(t, ids[fmt.Sprintf("T-%03d", i)], "T-%03d must be present", i)
	}
}

func TestUpdate_ConcurrentUpdates_SameTask(t *testing.T) {
	// Multiple goroutines updating the same task must not corrupt the file.
	// Final state should reflect exactly one entry for the task.
	path := writeTempState(t, "")
	sm := NewStateManager(path)

	const goroutines = 10
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sm.UpdateStatus("T-001", StatusInProgress, "claude")
		}()
	}
	wg.Wait()

	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, 1, "concurrent updates to the same task must result in exactly one entry")
	assert.Equal(t, "T-001", states[0].TaskID)
}

// ---- Large file integration test --------------------------------------------

func TestLoad_LargeFile(t *testing.T) {
	t.Parallel()

	const n = 150
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString(fmt.Sprintf("T-%03d|not_started|||\n", i))
	}

	path := writeTempState(t, sb.String())
	sm := NewStateManager(path)
	states, err := sm.Load()
	require.NoError(t, err)
	assert.Len(t, states, n)
}

func TestLoad_LargeFile_AllEntriesIntact(t *testing.T) {
	t.Parallel()

	const n = 100
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString(fmt.Sprintf("T-%03d|not_started|||\n", i))
	}

	path := writeTempState(t, sb.String())
	sm := NewStateManager(path)

	states, err := sm.Load()
	require.NoError(t, err)
	require.Len(t, states, n)

	// Sort by task ID for deterministic comparison.
	sort.Slice(states, func(i, j int) bool {
		return states[i].TaskID < states[j].TaskID
	})
	for i, s := range states {
		expectedID := fmt.Sprintf("T-%03d", i+1)
		assert.Equal(t, expectedID, s.TaskID)
		assert.Equal(t, StatusNotStarted, s.Status)
	}
}

// ---- Benchmark tests --------------------------------------------------------

func BenchmarkLoad_LargeFile(b *testing.B) {
	const n = 500
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString(fmt.Sprintf("T-%03d|not_started|||\n", i))
	}
	dir := b.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	require.NoError(b, os.WriteFile(path, []byte(sb.String()), 0644))
	sm := NewStateManager(path)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := sm.Load()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUpdate_LargeFile(b *testing.B) {
	const n = 500
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString(fmt.Sprintf("T-%03d|not_started|||\n", i))
	}
	dir := b.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	require.NoError(b, os.WriteFile(path, []byte(sb.String()), 0644))
	sm := NewStateManager(path)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		taskID := fmt.Sprintf("T-%03d", (i%n)+1)
		if err := sm.UpdateStatus(taskID, StatusInProgress, "claude"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseLine(b *testing.B) {
	line := "T-001|completed|claude|2026-02-17T10:30:00Z|Foundation setup done"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parseLine(line)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFormatLine(b *testing.B) {
	ts, _ := time.Parse(time.RFC3339, "2026-02-17T10:30:00Z")
	state := TaskState{
		TaskID:    "T-001",
		Status:    StatusCompleted,
		Agent:     "claude",
		Timestamp: ts,
		Notes:     "Foundation setup done",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = formatLine(state)
	}
}

// ---- Helpers ----------------------------------------------------------------

// fixtureStatePath returns the absolute path to a file inside
// internal/task/testdata/state/.
func fixtureStatePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", "state", name)
}

// writeTempState writes content to a temp file and returns its path.
func writeTempState(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

// mustParseRFC3339 parses an RFC3339 time string and panics on failure.
func mustParseRFC3339(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
