package task

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- ParsePhaseLine tests ---------------------------------------------------

func TestParsePhaseLine_FourFields(t *testing.T) {
	t.Parallel()

	p, err := ParsePhaseLine("1|Foundation & Setup|T-001|T-010")
	require.NoError(t, err)
	assert.Equal(t, 1, p.ID)
	assert.Equal(t, "Foundation & Setup", p.Name)
	assert.Equal(t, "T-001", p.StartTask)
	assert.Equal(t, "T-010", p.EndTask)
}

func TestParsePhaseLine_SixFields(t *testing.T) {
	t.Parallel()

	p, err := ParsePhaseLine("2|task-system-agents|Task System & Agent Adapters|016|030|🤖")
	require.NoError(t, err)
	assert.Equal(t, 2, p.ID)
	assert.Equal(t, "Task System & Agent Adapters", p.Name)
	assert.Equal(t, "T-016", p.StartTask)
	assert.Equal(t, "T-030", p.EndTask)
}

func TestParsePhaseLine_WhitespaceTrimmed(t *testing.T) {
	t.Parallel()

	p, err := ParsePhaseLine("  1 | Foundation & Setup | T-001 | T-010 ")
	require.NoError(t, err)
	assert.Equal(t, 1, p.ID)
	assert.Equal(t, "Foundation & Setup", p.Name)
	assert.Equal(t, "T-001", p.StartTask)
	assert.Equal(t, "T-010", p.EndTask)
}

func TestParsePhaseLine_FiveFieldSixFieldFormat(t *testing.T) {
	t.Parallel()

	// Five-field in six-field format: ID|slug|DisplayName|StartNum|EndNum
	// (no icon field -- should still parse correctly).
	p, err := ParsePhaseLine("1|foundation|Foundation|001|015")
	require.NoError(t, err)
	assert.Equal(t, 1, p.ID)
	assert.Equal(t, "Foundation", p.Name)
	assert.Equal(t, "T-001", p.StartTask)
	assert.Equal(t, "T-015", p.EndTask)
}

func TestParsePhaseLine_TooFewFields(t *testing.T) {
	t.Parallel()

	_, err := ParsePhaseLine("1|Foundation")
	assert.Error(t, err)
}

func TestParsePhaseLine_NonNumericID(t *testing.T) {
	t.Parallel()

	_, err := ParsePhaseLine("abc|Foundation & Setup|T-001|T-010")
	assert.Error(t, err)
}

func TestParsePhaseLine_ZeroPaddedNumericStart(t *testing.T) {
	t.Parallel()

	// Six-field format with zero-padded start/end numbers.
	p, err := ParsePhaseLine("1|foundation|Foundation|001|015|🏗")
	require.NoError(t, err)
	assert.Equal(t, "T-001", p.StartTask)
	assert.Equal(t, "T-015", p.EndTask)
}

func TestParsePhaseLine_SingleTask(t *testing.T) {
	t.Parallel()

	// Phase where StartTask == EndTask (single task phase).
	p, err := ParsePhaseLine("5|Single Task Phase|T-042|T-042")
	require.NoError(t, err)
	assert.Equal(t, "T-042", p.StartTask)
	assert.Equal(t, "T-042", p.EndTask)
}

func TestParsePhaseLine_AllSixFieldVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		line      string
		wantID    int
		wantName  string
		wantStart string
		wantEnd   string
	}{
		{
			name:      "foundation phase",
			line:      "1|foundation|Foundation|001|015|🏗",
			wantID:    1,
			wantName:  "Foundation",
			wantStart: "T-001",
			wantEnd:   "T-015",
		},
		{
			name:      "large phase range",
			line:      "7|polish-distribution|Polish & Distribution|079|087|🚀",
			wantID:    7,
			wantName:  "Polish & Distribution",
			wantStart: "T-079",
			wantEnd:   "T-087",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, err := ParsePhaseLine(tt.line)
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, p.ID)
			assert.Equal(t, tt.wantName, p.Name)
			assert.Equal(t, tt.wantStart, p.StartTask)
			assert.Equal(t, tt.wantEnd, p.EndTask)
		})
	}
}

func TestParsePhaseLine_NonNumericTaskStartInSixField(t *testing.T) {
	t.Parallel()

	_, err := ParsePhaseLine("1|foundation|Foundation|abc|015|🏗")
	assert.Error(t, err)
}

func TestParsePhaseLine_NonNumericTaskEndInSixField(t *testing.T) {
	t.Parallel()

	_, err := ParsePhaseLine("1|foundation|Foundation|001|xyz|🏗")
	assert.Error(t, err)
}

// ---- LoadPhases tests -------------------------------------------------------

func TestLoadPhases_FourFieldFixture(t *testing.T) {
	t.Parallel()

	phases, err := LoadPhases(fixturePhasespath(t, "valid-4field.conf"))
	require.NoError(t, err)
	require.Len(t, phases, 3)

	assert.Equal(t, 1, phases[0].ID)
	assert.Equal(t, "Foundation & Setup", phases[0].Name)
	assert.Equal(t, "T-001", phases[0].StartTask)
	assert.Equal(t, "T-010", phases[0].EndTask)

	assert.Equal(t, 2, phases[1].ID)
	assert.Equal(t, 3, phases[2].ID)
}

func TestLoadPhases_SixFieldFixture(t *testing.T) {
	t.Parallel()

	phases, err := LoadPhases(fixturePhasespath(t, "valid-6field.conf"))
	require.NoError(t, err)
	require.Len(t, phases, 3)

	assert.Equal(t, 1, phases[0].ID)
	assert.Equal(t, "Foundation", phases[0].Name)
	assert.Equal(t, "T-001", phases[0].StartTask)
	assert.Equal(t, "T-015", phases[0].EndTask)
}

func TestLoadPhases_EmptyFile(t *testing.T) {
	t.Parallel()

	phases, err := LoadPhases(fixturePhasespath(t, "empty.conf"))
	require.NoError(t, err)
	assert.Empty(t, phases)
}

func TestLoadPhases_NonExistentFile(t *testing.T) {
	t.Parallel()

	_, err := LoadPhases("/tmp/raven-test-nonexistent-phases.conf")
	assert.Error(t, err)
}

func TestLoadPhases_SortedByID(t *testing.T) {
	t.Parallel()

	phases, err := LoadPhases(fixturePhasespath(t, "unordered.conf"))
	require.NoError(t, err)
	require.Len(t, phases, 3)

	// Must be sorted ascending by ID regardless of file order.
	assert.Equal(t, 1, phases[0].ID)
	assert.Equal(t, 2, phases[1].ID)
	assert.Equal(t, 3, phases[2].ID)
}

func TestLoadPhases_SkipsCommentsAndBlanks(t *testing.T) {
	t.Parallel()

	// valid-4field.conf has comment lines and no blank lines; load it and
	// verify only data lines are returned.
	phases, err := LoadPhases(fixturePhasespath(t, "valid-4field.conf"))
	require.NoError(t, err)
	// The fixture has 3 data lines.
	assert.Len(t, phases, 3)
}

func TestLoadPhases_RealisticMultiPhaseFile(t *testing.T) {
	t.Parallel()

	// Write a realistic phases.conf to a temp file — mirrors the structure of
	// the real project file without depending on a gitignored artifact.
	content := `# Raven Phase Definitions
#
# Format: ID|SLUG|DISPLAY_NAME|TASK_START|TASK_END|ICON
#
1|foundation|Foundation|001|015|x
2|task-system-agents|Task System & Agent Adapters|016|030|x
3|review-pipeline|Review Pipeline|031|042|x
4|workflow-engine|Workflow Engine & Pipeline|043|055|x
5|prd-decomposition|PRD Decomposition|056|065|x
6|tui-command-center|TUI Command Center|066|078|x
7|polish-distribution|Polish & Distribution|079|087|x
8|headless-observability|Headless Observability|088|089|x
`
	path := filepath.Join(t.TempDir(), "phases.conf")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	phases, err := LoadPhases(path)
	require.NoError(t, err)
	assert.Len(t, phases, 8, "should load all 8 phases")

	// Verify phases are sorted by ID.
	for i := 1; i < len(phases); i++ {
		assert.Less(t, phases[i-1].ID, phases[i].ID, "phases must be sorted by ID")
	}

	// Validate non-overlapping ranges.
	err = ValidatePhases(phases)
	assert.NoError(t, err, "phases should be valid (no overlaps)")
}

// ---- PhaseForTask tests -----------------------------------------------------

func TestPhaseForTask_TaskInFirstPhase(t *testing.T) {
	t.Parallel()

	phases := samplePhases()

	p := PhaseForTask(phases, "T-005")
	require.NotNil(t, p)
	assert.Equal(t, 1, p.ID)
}

func TestPhaseForTask_TaskInSecondPhase(t *testing.T) {
	t.Parallel()

	phases := samplePhases()

	p := PhaseForTask(phases, "T-015")
	require.NotNil(t, p)
	assert.Equal(t, 2, p.ID)
}

func TestPhaseForTask_TaskOutsideAllPhases(t *testing.T) {
	t.Parallel()

	phases := samplePhases()

	p := PhaseForTask(phases, "T-099")
	assert.Nil(t, p)
}

func TestPhaseForTask_BoundaryStartTask(t *testing.T) {
	t.Parallel()

	phases := samplePhases()

	// T-001 is exactly at the start of phase 1.
	p := PhaseForTask(phases, "T-001")
	require.NotNil(t, p)
	assert.Equal(t, 1, p.ID)
}

func TestPhaseForTask_BoundaryEndTask(t *testing.T) {
	t.Parallel()

	phases := samplePhases()

	// T-010 is exactly at the end of phase 1.
	p := PhaseForTask(phases, "T-010")
	require.NotNil(t, p)
	assert.Equal(t, 1, p.ID)
}

func TestPhaseForTask_InvalidTaskID(t *testing.T) {
	t.Parallel()

	phases := samplePhases()

	// An invalid task ID that cannot be parsed returns nil.
	p := PhaseForTask(phases, "invalid")
	assert.Nil(t, p)
}

// ---- PhaseByID tests --------------------------------------------------------

func TestPhaseByID_ExistingPhase(t *testing.T) {
	t.Parallel()

	phases := samplePhases()

	p := PhaseByID(phases, 1)
	require.NotNil(t, p)
	assert.Equal(t, "Foundation & Setup", p.Name)
}

func TestPhaseByID_NonExistentPhase(t *testing.T) {
	t.Parallel()

	phases := samplePhases()

	p := PhaseByID(phases, 99)
	assert.Nil(t, p)
}

func TestPhaseByID_AllPhases(t *testing.T) {
	t.Parallel()

	phases := samplePhases()

	for _, want := range phases {
		want := want
		t.Run(want.Name, func(t *testing.T) {
			t.Parallel()
			got := PhaseByID(phases, want.ID)
			require.NotNil(t, got)
			assert.Equal(t, want.ID, got.ID)
		})
	}
}

// ---- TaskIDNumber tests -----------------------------------------------------

func TestTaskIDNumber_Standard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		taskID string
		want   int
	}{
		{"T-016", 16},
		{"T-001", 1},
		{"T-100", 100},
		{"T-087", 87},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.taskID, func(t *testing.T) {
			t.Parallel()
			got, err := TaskIDNumber(tt.taskID)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTaskIDNumber_InvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		taskID string
	}{
		{name: "no prefix", taskID: "invalid"},
		{name: "empty string", taskID: ""},
		{name: "wrong prefix", taskID: "X-001"},
		{name: "T- only", taskID: "T-"},
		{name: "non-numeric suffix", taskID: "T-abc"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := TaskIDNumber(tt.taskID)
			assert.Error(t, err)
		})
	}
}

// ---- TasksInPhase tests -----------------------------------------------------

func TestTasksInPhase_SmallRange(t *testing.T) {
	t.Parallel()

	phase := Phase{ID: 1, Name: "Test", StartTask: "T-001", EndTask: "T-003"}
	got := TasksInPhase(phase)
	assert.Equal(t, []string{"T-001", "T-002", "T-003"}, got)
}

func TestTasksInPhase_SingleTask(t *testing.T) {
	t.Parallel()

	phase := Phase{ID: 1, Name: "Test", StartTask: "T-042", EndTask: "T-042"}
	got := TasksInPhase(phase)
	assert.Equal(t, []string{"T-042"}, got)
}

func TestTasksInPhase_StartAfterEnd(t *testing.T) {
	t.Parallel()

	// Invalid range: returns empty slice without error.
	phase := Phase{ID: 1, Name: "Test", StartTask: "T-010", EndTask: "T-001"}
	got := TasksInPhase(phase)
	assert.Empty(t, got)
}

func TestTasksInPhase_LargeRange(t *testing.T) {
	t.Parallel()

	phase := Phase{ID: 1, Name: "Test", StartTask: "T-001", EndTask: "T-100"}
	got := TasksInPhase(phase)
	assert.Len(t, got, 100)
	assert.Equal(t, "T-001", got[0])
	assert.Equal(t, "T-100", got[99])
}

func TestTasksInPhase_ZeroPaddedIDs(t *testing.T) {
	t.Parallel()

	phase := Phase{ID: 1, Name: "Test", StartTask: "T-008", EndTask: "T-012"}
	got := TasksInPhase(phase)
	assert.Equal(t, []string{"T-008", "T-009", "T-010", "T-011", "T-012"}, got)
}

func TestTasksInPhase_InvalidStartTask(t *testing.T) {
	t.Parallel()

	phase := Phase{ID: 1, Name: "Test", StartTask: "invalid", EndTask: "T-010"}
	got := TasksInPhase(phase)
	assert.Empty(t, got)
}

// ---- FormatPhaseLine tests --------------------------------------------------

func TestFormatPhaseLine_RoundTrip(t *testing.T) {
	t.Parallel()

	original := Phase{
		ID:        1,
		Name:      "Foundation & Setup",
		StartTask: "T-001",
		EndTask:   "T-010",
	}

	line := FormatPhaseLine(original)
	assert.Equal(t, "1|Foundation & Setup|T-001|T-010", line)

	// Round-trip: parse the formatted line back and compare.
	parsed, err := ParsePhaseLine(line)
	require.NoError(t, err)
	assert.Equal(t, original.ID, parsed.ID)
	assert.Equal(t, original.Name, parsed.Name)
	assert.Equal(t, original.StartTask, parsed.StartTask)
	assert.Equal(t, original.EndTask, parsed.EndTask)
}

func TestFormatPhaseLine_AllSamplePhases(t *testing.T) {
	t.Parallel()

	for _, p := range samplePhases() {
		p := p
		t.Run(p.Name, func(t *testing.T) {
			t.Parallel()
			line := FormatPhaseLine(p)
			parsed, err := ParsePhaseLine(line)
			require.NoError(t, err)
			assert.Equal(t, p.ID, parsed.ID)
			assert.Equal(t, p.Name, parsed.Name)
			assert.Equal(t, p.StartTask, parsed.StartTask)
			assert.Equal(t, p.EndTask, parsed.EndTask)
		})
	}
}

// ---- ValidatePhases tests ---------------------------------------------------

func TestValidatePhases_ValidPhases(t *testing.T) {
	t.Parallel()

	err := ValidatePhases(samplePhases())
	assert.NoError(t, err)
}

func TestValidatePhases_OverlappingRanges(t *testing.T) {
	t.Parallel()

	phases, err := LoadPhases(fixturePhasespath(t, "overlapping.conf"))
	require.NoError(t, err)

	err = ValidatePhases(phases)
	assert.Error(t, err, "overlapping phases must be detected")
}

func TestValidatePhases_DuplicateID(t *testing.T) {
	t.Parallel()

	phases := []Phase{
		{ID: 1, Name: "Phase One", StartTask: "T-001", EndTask: "T-010"},
		{ID: 1, Name: "Phase Duplicate", StartTask: "T-011", EndTask: "T-020"},
	}
	err := ValidatePhases(phases)
	assert.Error(t, err, "duplicate phase ID must be detected")
}

func TestValidatePhases_StartAfterEnd(t *testing.T) {
	t.Parallel()

	phases := []Phase{
		{ID: 1, Name: "Phase One", StartTask: "T-010", EndTask: "T-001"},
	}
	err := ValidatePhases(phases)
	assert.Error(t, err, "StartTask > EndTask must be detected")
}

func TestValidatePhases_EmptyName(t *testing.T) {
	t.Parallel()

	phases := []Phase{
		{ID: 1, Name: "", StartTask: "T-001", EndTask: "T-010"},
	}
	err := ValidatePhases(phases)
	assert.Error(t, err, "empty phase name must be detected")
}

func TestValidatePhases_ZeroID(t *testing.T) {
	t.Parallel()

	phases := []Phase{
		{ID: 0, Name: "Phase Zero", StartTask: "T-001", EndTask: "T-010"},
	}
	err := ValidatePhases(phases)
	assert.Error(t, err, "phase ID 0 must be rejected")
}

func TestValidatePhases_EmptySlice(t *testing.T) {
	t.Parallel()

	err := ValidatePhases([]Phase{})
	assert.NoError(t, err, "empty phases slice is valid")
}

func TestValidatePhases_InvalidStartTask(t *testing.T) {
	t.Parallel()

	phases := []Phase{
		{ID: 1, Name: "Phase One", StartTask: "INVALID", EndTask: "T-010"},
	}
	err := ValidatePhases(phases)
	assert.Error(t, err)
}

// ---- Helpers ----------------------------------------------------------------

// fixturePhasespath returns the absolute path to a file inside
// internal/task/testdata/phases/.
func fixturePhasespath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", "phases", name)
}

// samplePhases returns a standard 3-phase slice used by multiple tests.
func samplePhases() []Phase {
	return []Phase{
		{ID: 1, Name: "Foundation & Setup", StartTask: "T-001", EndTask: "T-010"},
		{ID: 2, Name: "Core Implementation", StartTask: "T-011", EndTask: "T-020"},
		{ID: 3, Name: "Review Pipeline", StartTask: "T-021", EndTask: "T-030"},
	}
}
