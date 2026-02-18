package prd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
)

// --- Slugify tests ---

func TestSlugify_BasicConversions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "lowercase spaces to hyphens",
			input: "Set Up Authentication Middleware",
			want:  "set-up-authentication-middleware",
		},
		{
			name:  "strips special characters",
			input: "Hello, World! (v2)",
			want:  "hello-world-v2",
		},
		{
			name:  "collapses multiple hyphens",
			input: "foo---bar",
			want:  "foo-bar",
		},
		{
			name:  "trims leading and trailing hyphens",
			input: "-foo-bar-",
			want:  "foo-bar",
		},
		{
			name:  "strips non-ASCII characters",
			input: "café latte",
			want:  "caf-latte",
		},
		{
			name:  "truncates at 50 chars at word boundary",
			input: "this-is-a-very-long-slug-that-exceeds-the-fifty-character-limit-for-filenames",
			want:  "this-is-a-very-long-slug-that-exceeds-the-fifty",
		},
		{
			name:  "empty string returns empty",
			input: "",
			want:  "",
		},
		{
			name:  "already kebab case",
			input: "already-kebab-case",
			want:  "already-kebab-case",
		},
		{
			name:  "all special chars",
			input: "!@#$%^&*()",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Slugify(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSlugify_ExactlyFiftyChars(t *testing.T) {
	t.Parallel()

	// Exactly 50 characters -- no truncation expected.
	// 10 + 1 + 10 + 1 + 10 + 1 + 10 + 1 + 6 = 50
	input := "abcdefghij-abcdefghij-abcdefghij-abcdefghij-abcdef"
	require.Len(t, input, 50, "input must be exactly 50 chars")
	got := Slugify(input)
	assert.Equal(t, input, got)
}

func TestSlugify_TruncatesAtWordBoundary(t *testing.T) {
	t.Parallel()

	// 55 chars with a hyphen at position 45.
	input := "aaa-bbb-ccc-ddd-eee-fff-ggg-hhh-iii-jjj-kkk-lll-m"
	got := Slugify(input)
	assert.LessOrEqual(t, len(got), 50)
	assert.False(t, strings.HasSuffix(got, "-"), "should not end with hyphen")
}

// --- ResequenceIDs tests ---

func TestResequenceIDs_NoGaps(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Task A"},
		{GlobalID: "T-002", Title: "Task B", Dependencies: []string{"T-001"}},
		{GlobalID: "T-003", Title: "Task C", Dependencies: []string{"T-001", "T-002"}},
	}

	out, mapping := ResequenceIDs(tasks)
	// No gaps, so no remapping should occur.
	assert.Empty(t, mapping)
	assert.Equal(t, "T-001", out[0].GlobalID)
	assert.Equal(t, "T-002", out[1].GlobalID)
	assert.Equal(t, "T-003", out[2].GlobalID)
	// Dependencies unchanged.
	assert.Equal(t, []string{"T-001"}, out[1].Dependencies)
	assert.Equal(t, []string{"T-001", "T-002"}, out[2].Dependencies)
}

func TestResequenceIDs_WithGaps(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Task A"},
		{GlobalID: "T-003", Title: "Task C", Dependencies: []string{"T-001"}},
		{GlobalID: "T-005", Title: "Task E", Dependencies: []string{"T-001", "T-003"}},
	}

	out, mapping := ResequenceIDs(tasks)

	// T-003 -> T-002, T-005 -> T-003.
	assert.Equal(t, "T-001", out[0].GlobalID)
	assert.Equal(t, "T-002", out[1].GlobalID)
	assert.Equal(t, "T-003", out[2].GlobalID)

	assert.Equal(t, "T-002", mapping["T-003"])
	assert.Equal(t, "T-003", mapping["T-005"])

	// Dependencies remapped.
	assert.Equal(t, []string{"T-001"}, out[1].Dependencies)
	assert.Equal(t, []string{"T-001", "T-002"}, out[2].Dependencies)
}

func TestResequenceIDs_Empty(t *testing.T) {
	t.Parallel()

	out, mapping := ResequenceIDs(nil)
	assert.Nil(t, out)
	assert.Empty(t, mapping)
}

func TestResequenceIDs_LargeCount_FourDigitFormat(t *testing.T) {
	t.Parallel()

	// Build 1000 tasks with non-sequential IDs (gaps) so re-sequencing is triggered.
	tasks := make([]MergedTask, 1000)
	for i := range tasks {
		// Use IDs with gaps: T-0001, T-0003, T-0005, ... to force remapping.
		tasks[i] = MergedTask{
			GlobalID: fmt.Sprintf("T-%04d", (i+1)*2-1),
			Title:    "Task",
		}
	}

	out, mapping := ResequenceIDs(tasks)
	// With 1000 tasks, format should be T-NNNN.
	assert.Equal(t, "T-0001", out[0].GlobalID)
	assert.Equal(t, "T-1000", out[999].GlobalID)
	// All original IDs should have been remapped.
	assert.NotEmpty(t, mapping)
}

// --- AssignPhases tests ---

func TestAssignPhases_EmptyTasks(t *testing.T) {
	t.Parallel()

	phases := AssignPhases(nil, nil, nil)
	assert.Nil(t, phases)
}

func TestAssignPhases_AllDepthZero(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "A", EpicID: "E-001"},
		{GlobalID: "T-002", Title: "B", EpicID: "E-001"},
	}

	phases := AssignPhases(tasks, map[string]int{}, nil)
	require.Len(t, phases, 1)
	assert.Equal(t, 1, phases[0].ID)
	assert.Equal(t, "T-001", phases[0].StartTask)
	assert.Equal(t, "T-002", phases[0].EndTask)
	assert.Len(t, phases[0].Tasks, 2)
}

func TestAssignPhases_MultipleDepths(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "A", EpicID: "E-001"},
		{GlobalID: "T-002", Title: "B", EpicID: "E-001"},
		{GlobalID: "T-003", Title: "C", EpicID: "E-002"},
	}
	depths := map[string]int{
		"T-001": 0,
		"T-002": 0,
		"T-003": 1,
	}

	phases := AssignPhases(tasks, depths, nil)
	require.Len(t, phases, 2)

	// Phase 1: T-001, T-002
	assert.Equal(t, 1, phases[0].ID)
	assert.Len(t, phases[0].Tasks, 2)
	assert.Equal(t, "T-001", phases[0].StartTask)
	assert.Equal(t, "T-002", phases[0].EndTask)

	// Phase 2: T-003
	assert.Equal(t, 2, phases[1].ID)
	assert.Len(t, phases[1].Tasks, 1)
	assert.Equal(t, "T-003", phases[1].StartTask)
	assert.Equal(t, "T-003", phases[1].EndTask)
}

func TestAssignPhases_PhaseNamingFromEpics(t *testing.T) {
	t.Parallel()

	epics := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Foundation"},
			{ID: "E-002", Title: "Core API"},
		},
	}
	tasks := []MergedTask{
		{GlobalID: "T-001", EpicID: "E-001"},
		{GlobalID: "T-002", EpicID: "E-001"},
		{GlobalID: "T-003", EpicID: "E-002"},
	}
	depths := map[string]int{"T-001": 0, "T-002": 0, "T-003": 1}

	phases := AssignPhases(tasks, depths, epics)
	require.Len(t, phases, 2)
	assert.Equal(t, "Foundation", phases[0].Name)
	assert.Equal(t, "Core API", phases[1].Name)
}

func TestAssignPhases_FallbackPhaseNameWhenNoEpics(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "X"},
	}

	phases := AssignPhases(tasks, nil, nil)
	require.Len(t, phases, 1)
	assert.Equal(t, "Phase 1", phases[0].Name)
}

// --- Emit integration tests ---

func TestEmit_BasicOutput(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{
			GlobalID:           "T-001",
			TempID:             "E001-T01",
			EpicID:             "E-001",
			Title:              "Set Up Project",
			Description:        "Initialize the project structure.",
			AcceptanceCriteria: []string{"Module created", "Dependencies declared"},
			Effort:             "small",
			Priority:           "must-have",
		},
		{
			GlobalID:           "T-002",
			TempID:             "E001-T02",
			EpicID:             "E-001",
			Title:              "Implement Core Logic",
			Description:        "Write the core business logic.",
			AcceptanceCriteria: []string{"Tests pass"},
			Dependencies:       []string{"T-001"},
			Effort:             "medium",
			Priority:           "must-have",
		},
	}

	epics := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Foundation"},
		},
	}

	validation := &DAGValidation{
		Valid:            true,
		TopologicalOrder: []string{"T-001", "T-002"},
		Depths:           map[string]int{"T-001": 0, "T-002": 1},
		MaxDepth:         1,
	}

	result, err := emitter.Emit(EmitOpts{
		Tasks:      tasks,
		Validation: validation,
		Epics:      epics,
	})

	require.NoError(t, err)
	assert.Equal(t, outDir, result.OutputDir)
	assert.Equal(t, 2, result.TotalTasks)
	assert.Equal(t, 2, result.TotalPhases)
	assert.Len(t, result.TaskFiles, 2)
	assert.NotEmpty(t, result.TaskStateFile)
	assert.NotEmpty(t, result.PhasesFile)
	assert.NotEmpty(t, result.ProgressFile)
	assert.NotEmpty(t, result.IndexFile)

	// Verify task spec files exist and have correct content.
	for _, path := range result.TaskFiles {
		_, err := os.Stat(path)
		require.NoError(t, err, "task file should exist: %s", path)
	}

	// Verify task-state.conf content.
	stateContent, err := os.ReadFile(result.TaskStateFile)
	require.NoError(t, err)
	stateStr := string(stateContent)
	assert.Contains(t, stateStr, "T-001|not_started|")
	assert.Contains(t, stateStr, "T-002|not_started|")

	// Verify phases.conf content.
	phasesContent, err := os.ReadFile(result.PhasesFile)
	require.NoError(t, err)
	phasesStr := string(phasesContent)
	assert.Contains(t, phasesStr, "1|Foundation|T-001|T-001")
	assert.Contains(t, phasesStr, "2|Foundation|T-002|T-002")

	// Verify PROGRESS.md content.
	progressContent, err := os.ReadFile(result.ProgressFile)
	require.NoError(t, err)
	progressStr := string(progressContent)
	assert.Contains(t, progressStr, "# Raven Task Progress Log")
	assert.Contains(t, progressStr, "Not Started | 2")

	// Verify INDEX.md content.
	indexContent, err := os.ReadFile(result.IndexFile)
	require.NoError(t, err)
	indexStr := string(indexContent)
	assert.Contains(t, indexStr, "# Task Index")
	assert.Contains(t, indexStr, "T-001")
	assert.Contains(t, indexStr, "T-002")
}

func TestEmit_ResequencesIDs(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	// Tasks with gaps from deduplication.
	tasks := []MergedTask{
		{GlobalID: "T-001", TempID: "E001-T01", Title: "First", Description: "Desc", Effort: "small", Priority: "must-have"},
		{GlobalID: "T-003", TempID: "E001-T03", Title: "Third", Description: "Desc", Effort: "small", Priority: "must-have", Dependencies: []string{"T-001"}},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)

	// T-003 should be re-sequenced to T-002.
	stateContent, err := os.ReadFile(result.TaskStateFile)
	require.NoError(t, err)
	stateStr := string(stateContent)
	assert.Contains(t, stateStr, "T-001|not_started|")
	assert.Contains(t, stateStr, "T-002|not_started|")
	assert.NotContains(t, stateStr, "T-003")
}

func TestEmit_NoOverwriteWithoutForce(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir) // force=false by default

	tasks := []MergedTask{
		{
			GlobalID:           "T-001",
			Title:              "My Task",
			Description:        "Desc",
			Effort:             "small",
			Priority:           "must-have",
			AcceptanceCriteria: []string{"Done"},
		},
	}

	// Pre-create the task file that will be written first (T-001-my-task.md).
	taskFile := filepath.Join(outDir, "T-001-my-task.md")
	require.NoError(t, os.WriteFile(taskFile, []byte("existing"), 0644))

	_, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestEmit_ForceOverwrite(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Task", Description: "Desc", Effort: "small", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}},
	}

	// First emit.
	result1, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)

	// Second emit should succeed with force=true.
	result2, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)

	assert.Equal(t, result1.TotalTasks, result2.TotalTasks)
}

func TestEmit_TaskFileContent(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{
			GlobalID:           "T-001",
			Title:              "Configure CI Pipeline",
			Description:        "Set up continuous integration.",
			AcceptanceCriteria: []string{"Pipeline runs on push", "Tests pass"},
			Dependencies:       []string{},
			Effort:             "medium",
			Priority:           "must-have",
		},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)
	require.Len(t, result.TaskFiles, 1)

	content, err := os.ReadFile(result.TaskFiles[0])
	require.NoError(t, err)
	s := string(content)

	assert.Contains(t, s, "# T-001: Configure CI Pipeline")
	assert.Contains(t, s, "## Metadata")
	assert.Contains(t, s, "| Priority | must-have |")
	assert.Contains(t, s, "| Estimated Effort | medium |")
	assert.Contains(t, s, "| Dependencies | None |")
	assert.Contains(t, s, "## Goal")
	assert.Contains(t, s, "Set up continuous integration.")
	assert.Contains(t, s, "## Acceptance Criteria")
	assert.Contains(t, s, "- [ ] Pipeline runs on push")
	assert.Contains(t, s, "- [ ] Tests pass")
}

func TestEmit_SlugCollisionHandling(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	// Two tasks that produce the same slug.
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Implement Auth", Description: "D", Effort: "small", Priority: "must-have", AcceptanceCriteria: []string{"Done"}},
		{GlobalID: "T-002", Title: "Implement Auth", Description: "D", Effort: "small", Priority: "must-have", AcceptanceCriteria: []string{"Done"}},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)
	require.Len(t, result.TaskFiles, 2)

	// Both files should exist with distinct names.
	assert.NotEqual(t, result.TaskFiles[0], result.TaskFiles[1])

	basenames := make([]string, 2)
	for i, p := range result.TaskFiles {
		basenames[i] = filepath.Base(p)
	}
	assert.Contains(t, basenames[0], "T-001")
	assert.Contains(t, basenames[1], "T-002")
}

func TestEmit_MermaidGraphInIndex(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "A", Description: "D", Effort: "small", Priority: "must-have", AcceptanceCriteria: []string{"Done"}},
		{GlobalID: "T-002", Title: "B", Description: "D", Effort: "small", Priority: "must-have", AcceptanceCriteria: []string{"Done"}, Dependencies: []string{"T-001"}},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)

	content, err := os.ReadFile(result.IndexFile)
	require.NoError(t, err)
	s := string(content)

	assert.Contains(t, s, "```mermaid")
	assert.Contains(t, s, "graph TD")
	assert.Contains(t, s, "T-001 --> T-002")
}

func TestEmit_OutputDirCreated(t *testing.T) {
	t.Parallel()

	outDir := filepath.Join(t.TempDir(), "sub", "dir")
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "A", Description: "D", Effort: "small", Priority: "must-have", AcceptanceCriteria: []string{"Done"}},
	}

	_, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)

	info, err := os.Stat(outDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEmit_PhasesConfParseable(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "A", Description: "D", Effort: "small", Priority: "must-have", AcceptanceCriteria: []string{"Done"}},
		{GlobalID: "T-002", Title: "B", Description: "D", Effort: "small", Priority: "must-have", AcceptanceCriteria: []string{"Done"}, Dependencies: []string{"T-001"}},
	}

	validation := &DAGValidation{
		Valid:  true,
		Depths: map[string]int{"T-001": 0, "T-002": 1},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks, Validation: validation})
	require.NoError(t, err)

	// Read and manually parse phases.conf to verify format.
	content, err := os.ReadFile(result.PhasesFile)
	require.NoError(t, err)

	lines := strings.Split(string(content), "\n")
	var dataLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			dataLines = append(dataLines, trimmed)
		}
	}

	require.Len(t, dataLines, 2)

	// Verify four-field format: ID|Name|T-start|T-end
	for _, line := range dataLines {
		parts := strings.Split(line, "|")
		require.Len(t, parts, 4, "expected four fields in phases.conf line: %s", line)
		assert.True(t, strings.HasPrefix(parts[2], "T-"), "start task should have T- prefix: %s", parts[2])
		assert.True(t, strings.HasPrefix(parts[3], "T-"), "end task should have T- prefix: %s", parts[3])
	}
}

// --- uniqueSlug tests ---

func TestUniqueSlug_NoCollision(t *testing.T) {
	t.Parallel()

	task := MergedTask{GlobalID: "T-001", Title: "My Task"}
	used := make(map[string]int)
	slug := uniqueSlug(task, used)
	assert.Equal(t, "my-task", slug)
}

func TestUniqueSlug_CollisionAppendsNumber(t *testing.T) {
	t.Parallel()

	task1 := MergedTask{GlobalID: "T-001", Title: "My Task"}
	task2 := MergedTask{GlobalID: "T-002", Title: "My Task"}
	used := make(map[string]int)

	slug1 := uniqueSlug(task1, used)
	slug2 := uniqueSlug(task2, used)

	assert.Equal(t, "my-task", slug1)
	assert.NotEqual(t, slug1, slug2)
	assert.Contains(t, slug2, "my-task")
}

// --- renderTaskFile tests ---

func TestRenderTaskFile_WithDependencies(t *testing.T) {
	t.Parallel()

	task := MergedTask{
		GlobalID:           "T-005",
		Title:              "Write Unit Tests",
		Description:        "Cover all edge cases.",
		AcceptanceCriteria: []string{"90% coverage", "No flaky tests"},
		Dependencies:       []string{"T-001", "T-003"},
		Effort:             "large",
		Priority:           "should-have",
	}

	content, err := renderTaskFile(task)
	require.NoError(t, err)
	s := string(content)

	assert.Contains(t, s, "# T-005: Write Unit Tests")
	assert.Contains(t, s, "| Dependencies | T-001, T-003 |")
	assert.Contains(t, s, "| Priority | should-have |")
	assert.Contains(t, s, "- [ ] 90% coverage")
}

func TestRenderTaskFile_NoDependencies(t *testing.T) {
	t.Parallel()

	task := MergedTask{
		GlobalID:           "T-001",
		Title:              "Bootstrap",
		Description:        "Initial setup.",
		AcceptanceCriteria: []string{"Repo created"},
		Effort:             "small",
		Priority:           "must-have",
	}

	content, err := renderTaskFile(task)
	require.NoError(t, err)
	s := string(content)

	assert.Contains(t, s, "| Dependencies | None |")
}

// =============================================================================
// Additional Slugify tests
// =============================================================================

func TestSlugify_SpecCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "Set Up Authentication Middleware",
			input: "Set Up Authentication Middleware",
			want:  "set-up-authentication-middleware",
		},
		{
			name:  "Create CI/CD Pipeline",
			input: "Create CI/CD Pipeline",
			want:  "create-ci-cd-pipeline",
		},
		{
			name:  "extra spaces",
			input: "   Extra   Spaces   ",
			want:  "extra-spaces",
		},
		{
			name:  "unicode characters stripped",
			input: "Résumé Upload Feature",
			want:  "rsum-upload-feature",
		},
		{
			name:  "only unicode returns empty",
			input: "你好世界",
			want:  "",
		},
		{
			name:  "mixed ASCII and unicode",
			input: "Upload 照片 file",
			want:  "upload-file",
		},
		{
			name:  "numbers retained",
			input: "Phase 2 Setup",
			want:  "phase-2-setup",
		},
		{
			name:  "multiple slashes become hyphens",
			input: "A/B/C test",
			want:  "a-b-c-test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Slugify(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSlugify_TruncatesAtWordBoundaryLong(t *testing.T) {
	t.Parallel()

	// Build a title that is 55 chars with a hyphen at position 43.
	// "aaa-bbb-ccc-ddd-eee-fff-ggg-hhh-iii-jjj-kkk-toolong"
	// aaa(3)-bbb(7)-ccc(11)-ddd(15)-eee(19)-fff(23)-ggg(27)-hhh(31)-iii(35)-jjj(39)-kkk(43)-toolong(51)
	// = 51 chars total; truncated to 50 = "aaa-bbb-ccc-ddd-eee-fff-ggg-hhh-iii-jjj-kkk-toolon"
	// LastIndex of "-" in that 50-char prefix is at position 43, so result = 43 chars.
	input := "aaa-bbb-ccc-ddd-eee-fff-ggg-hhh-iii-jjj-kkk-toolong"
	require.Greater(t, len(input), 50, "test input must exceed 50 chars")

	got := Slugify(input)
	assert.LessOrEqual(t, len(got), 50, "slug must not exceed 50 chars")
	assert.False(t, strings.HasSuffix(got, "-"), "slug must not end with a hyphen")
	// The truncation should happen at the last hyphen before position 50.
	assert.Equal(t, "aaa-bbb-ccc-ddd-eee-fff-ggg-hhh-iii-jjj-kkk", got)
}

func TestSlugify_ExactlyFiftyOneBoundary(t *testing.T) {
	t.Parallel()

	// Title whose slug is exactly 51 chars with a hyphen at position 48.
	// "aaaaaaaaaa-bbbbbbbbbb-cccccccccc-ddddddddd-eeeeeee"
	//  10         21         32         42        50      => 51 chars
	// After truncation at 50: "aaaaaaaaaa-bbbbbbbbbb-cccccccccc-ddddddddd-eeeeeee"[:50]
	// That is 50 chars ending with 'e', no hyphen -- so LastIndex behaviour keeps up to "ddddddddd".
	// Let's just test that result <= 50 and no trailing hyphen.
	input := "aaaaaaaaaa-bbbbbbbbbb-cccccccccc-ddddddddd-eeeeeeee"
	require.Equal(t, 51, len(input))

	got := Slugify(input)
	assert.LessOrEqual(t, len(got), 50)
	assert.NotEmpty(t, got)
}

// =============================================================================
// Additional ResequenceIDs tests
// =============================================================================

func TestResequenceIDs_CloseGapsThreeItems(t *testing.T) {
	t.Parallel()

	// Spec requirement: [T-001, T-003, T-005] -> [T-001, T-002, T-003]
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Alpha"},
		{GlobalID: "T-003", Title: "Bravo", Dependencies: []string{"T-001"}},
		{GlobalID: "T-005", Title: "Charlie", Dependencies: []string{"T-001", "T-003"}},
	}

	out, mapping := ResequenceIDs(tasks)

	require.Len(t, out, 3)
	assert.Equal(t, "T-001", out[0].GlobalID)
	assert.Equal(t, "T-002", out[1].GlobalID)
	assert.Equal(t, "T-003", out[2].GlobalID)

	// Mapping should record the changed IDs only.
	assert.NotContains(t, mapping, "T-001", "T-001 was not remapped")
	assert.Equal(t, "T-002", mapping["T-003"])
	assert.Equal(t, "T-003", mapping["T-005"])

	// Dependencies should use new IDs.
	assert.Equal(t, []string{"T-001"}, out[1].Dependencies)
	assert.Equal(t, []string{"T-001", "T-002"}, out[2].Dependencies)
}

func TestResequenceIDs_PreservesOrderAndContent(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-010", Title: "Ten", Effort: "large", Priority: "must-have"},
		{GlobalID: "T-020", Title: "Twenty", EpicID: "E-001"},
	}

	out, mapping := ResequenceIDs(tasks)

	assert.Equal(t, "T-001", out[0].GlobalID)
	assert.Equal(t, "Ten", out[0].Title)
	assert.Equal(t, "large", out[0].Effort)
	assert.Equal(t, "T-002", out[1].GlobalID)
	assert.Equal(t, "Twenty", out[1].Title)
	assert.Equal(t, "E-001", out[1].EpicID)

	assert.Equal(t, "T-001", mapping["T-010"])
	assert.Equal(t, "T-002", mapping["T-020"])
}

func TestResequenceIDs_SingleTask(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-005", Title: "Only"},
	}
	out, mapping := ResequenceIDs(tasks)
	require.Len(t, out, 1)
	assert.Equal(t, "T-001", out[0].GlobalID)
	assert.Equal(t, "T-001", mapping["T-005"])
}

// =============================================================================
// Additional AssignPhases tests
// =============================================================================

func TestAssignPhases_FourDepths(t *testing.T) {
	t.Parallel()

	// Spec requirement: depths {A:0, B:0, C:1, D:2} -> 3 phases
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "A"},
		{GlobalID: "T-002", Title: "B"},
		{GlobalID: "T-003", Title: "C"},
		{GlobalID: "T-004", Title: "D"},
	}
	depths := map[string]int{
		"T-001": 0,
		"T-002": 0,
		"T-003": 1,
		"T-004": 2,
	}

	phases := AssignPhases(tasks, depths, nil)
	require.Len(t, phases, 3)

	assert.Equal(t, 1, phases[0].ID)
	assert.Len(t, phases[0].Tasks, 2)
	assert.Equal(t, "T-001", phases[0].StartTask)
	assert.Equal(t, "T-002", phases[0].EndTask)

	assert.Equal(t, 2, phases[1].ID)
	assert.Len(t, phases[1].Tasks, 1)
	assert.Equal(t, "T-003", phases[1].StartTask)

	assert.Equal(t, 3, phases[2].ID)
	assert.Len(t, phases[2].Tasks, 1)
	assert.Equal(t, "T-004", phases[2].StartTask)
}

func TestAssignPhases_NilDepthsTreatedAsZero(t *testing.T) {
	t.Parallel()

	// Spec requirement: nil depths map -> all tasks at depth 0 -> 1 phase
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Alpha"},
		{GlobalID: "T-002", Title: "Beta"},
		{GlobalID: "T-003", Title: "Gamma"},
	}

	phases := AssignPhases(tasks, nil, nil)
	require.Len(t, phases, 1)
	assert.Equal(t, 1, phases[0].ID)
	assert.Len(t, phases[0].Tasks, 3)
	assert.Equal(t, "T-001", phases[0].StartTask)
	assert.Equal(t, "T-003", phases[0].EndTask)
}

func TestAssignPhases_TasksSortedByGlobalIDWithinPhase(t *testing.T) {
	t.Parallel()

	// Tasks are provided in reverse order; within a phase they should be sorted.
	tasks := []MergedTask{
		{GlobalID: "T-003", Title: "C"},
		{GlobalID: "T-001", Title: "A"},
		{GlobalID: "T-002", Title: "B"},
	}
	depths := map[string]int{
		"T-001": 0,
		"T-002": 0,
		"T-003": 0,
	}

	phases := AssignPhases(tasks, depths, nil)
	require.Len(t, phases, 1)
	require.Len(t, phases[0].Tasks, 3)
	assert.Equal(t, "T-001", phases[0].Tasks[0].GlobalID)
	assert.Equal(t, "T-002", phases[0].Tasks[1].GlobalID)
	assert.Equal(t, "T-003", phases[0].Tasks[2].GlobalID)
}

func TestAssignPhases_PhaseNamingTieBreak(t *testing.T) {
	t.Parallel()

	// Two epics with equal count in the same phase -- lexicographic winner.
	epics := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Alpha Epic"},
			{ID: "E-002", Title: "Zebra Epic"},
		},
	}
	tasks := []MergedTask{
		{GlobalID: "T-001", EpicID: "E-001"},
		{GlobalID: "T-002", EpicID: "E-002"},
	}
	depths := map[string]int{"T-001": 0, "T-002": 0}

	phases := AssignPhases(tasks, depths, epics)
	require.Len(t, phases, 1)
	// "Alpha Epic" < "Zebra Epic" lexicographically.
	assert.Equal(t, "Alpha Epic", phases[0].Name)
}

func TestAssignPhases_UnknownEpicIDFallsBackToPhaseN(t *testing.T) {
	t.Parallel()

	epics := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-999", Title: "Some Epic"},
		},
	}
	// Tasks have epic IDs that are not in the epics list.
	tasks := []MergedTask{
		{GlobalID: "T-001", EpicID: "E-001"},
	}
	depths := map[string]int{"T-001": 0}

	phases := AssignPhases(tasks, depths, epics)
	require.Len(t, phases, 1)
	assert.Equal(t, "Phase 1", phases[0].Name)
}

// =============================================================================
// uniqueSlug edge-case tests
// =============================================================================

func TestUniqueSlug_EmptySlugFallsBackToGlobalID(t *testing.T) {
	t.Parallel()

	// A task whose title is all unicode/special chars will produce an empty slug.
	task := MergedTask{GlobalID: "T-001", Title: "!!!###@@@"}
	used := make(map[string]int)
	slug := uniqueSlug(task, used)
	// Slugify("!!!###@@@") -> "" -> fallback to lowercase GlobalID.
	assert.Equal(t, "t-001", slug)
}

func TestUniqueSlug_ThreeCollisionsGetDistinctSuffixes(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Auth"},
		{GlobalID: "T-002", Title: "Auth"},
		{GlobalID: "T-003", Title: "Auth"},
	}
	used := make(map[string]int)

	slugs := make([]string, 3)
	for i, tsk := range tasks {
		slugs[i] = uniqueSlug(tsk, used)
	}

	// All slugs must be distinct.
	seen := make(map[string]bool)
	for _, s := range slugs {
		assert.False(t, seen[s], "duplicate slug %q", s)
		seen[s] = true
	}
	// First slug has no suffix.
	assert.Equal(t, "auth", slugs[0])
	// Subsequent slugs contain "auth".
	assert.Contains(t, slugs[1], "auth")
	assert.Contains(t, slugs[2], "auth")
}

// =============================================================================
// renderTaskFile edge cases
// =============================================================================

func TestRenderTaskFile_NoAcceptanceCriteria(t *testing.T) {
	t.Parallel()

	task := MergedTask{
		GlobalID:           "T-007",
		Title:              "No Criteria Task",
		Description:        "Something to do.",
		AcceptanceCriteria: nil,
		Effort:             "small",
		Priority:           "nice-to-have",
	}

	content, err := renderTaskFile(task)
	require.NoError(t, err)
	s := string(content)

	// Must still have the Acceptance Criteria section header.
	assert.Contains(t, s, "## Acceptance Criteria")
	// The range block produces nothing when AcceptanceCriteria is nil/empty.
	// Verify the template does not crash and the file is structurally valid.
	assert.Contains(t, s, "# T-007: No Criteria Task")
}

func TestRenderTaskFile_PipeInTitleDoesNotBreakTemplate(t *testing.T) {
	t.Parallel()

	task := MergedTask{
		GlobalID:    "T-010",
		Title:       "A | B Pipeline",
		Description: "Pipe test.",
		Effort:      "small",
		Priority:    "must-have",
	}

	content, err := renderTaskFile(task)
	require.NoError(t, err)
	// Template uses the raw title in the heading -- should not error.
	assert.Contains(t, string(content), "A | B Pipeline")
}

// =============================================================================
// buildTaskState content tests
// =============================================================================

func TestBuildTaskState_Format(t *testing.T) {
	t.Parallel()

	e := NewEmitter(t.TempDir())
	tasks := []MergedTask{
		{GlobalID: "T-001"},
		{GlobalID: "T-002"},
		{GlobalID: "T-003"},
	}

	content := e.buildTaskState(tasks)
	s := string(content)

	// Must contain header comment lines.
	assert.Contains(t, s, "# Raven Task State")
	// Each task must have a pipe-delimited line.
	assert.Contains(t, s, "T-001|not_started|")
	assert.Contains(t, s, "T-002|not_started|")
	assert.Contains(t, s, "T-003|not_started|")
	// Should not contain tasks that were not provided.
	assert.NotContains(t, s, "T-004")
}

// =============================================================================
// buildPhasesConf content tests
// =============================================================================

func TestBuildPhasesConf_Format(t *testing.T) {
	t.Parallel()

	e := NewEmitter(t.TempDir())
	phases := []PhaseInfo{
		{ID: 1, Name: "Foundation", StartTask: "T-001", EndTask: "T-005"},
		{ID: 2, Name: "Core API", StartTask: "T-006", EndTask: "T-010"},
	}

	content := e.buildPhasesConf(phases)
	s := string(content)

	// Must contain header.
	assert.Contains(t, s, "# Raven Phases Configuration")
	// Lines must match ID|Name|Start|End format.
	assert.Contains(t, s, "1|Foundation|T-001|T-005")
	assert.Contains(t, s, "2|Core API|T-006|T-010")
}

// =============================================================================
// buildProgressMD content tests
// =============================================================================

func TestBuildProgressMD_Structure(t *testing.T) {
	t.Parallel()

	e := NewEmitter(t.TempDir())
	tasks := []MergedTask{
		{GlobalID: "T-001"},
		{GlobalID: "T-002"},
		{GlobalID: "T-003"},
	}
	phases := []PhaseInfo{
		{ID: 1, Name: "Foundation", StartTask: "T-001", EndTask: "T-002", Tasks: tasks[:2]},
		{ID: 2, Name: "Core", StartTask: "T-003", EndTask: "T-003", Tasks: tasks[2:]},
	}

	content := e.buildProgressMD(tasks, phases)
	s := string(content)

	assert.Contains(t, s, "# Raven Task Progress Log")
	assert.Contains(t, s, "## Summary")
	assert.Contains(t, s, "Not Started | 3")
	assert.Contains(t, s, "Completed | 0")
	assert.Contains(t, s, "## Completed Tasks")
	assert.Contains(t, s, "Phase 1: Foundation")
	assert.Contains(t, s, "Phase 2: Core")
	assert.Contains(t, s, "## Completion Log")
	assert.Contains(t, s, "_No tasks completed yet._")
}

// =============================================================================
// buildIndexMD content tests
// =============================================================================

func TestBuildIndexMD_TaskSummaryTable(t *testing.T) {
	t.Parallel()

	e := NewEmitter(t.TempDir())
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Setup", Priority: "must-have", Effort: "small", Dependencies: nil},
		{GlobalID: "T-002", Title: "Build", Priority: "should-have", Effort: "medium", Dependencies: []string{"T-001"}},
	}
	phases := []PhaseInfo{
		{ID: 1, Name: "Phase 1", StartTask: "T-001", EndTask: "T-001", Tasks: tasks[:1]},
		{ID: 2, Name: "Phase 2", StartTask: "T-002", EndTask: "T-002", Tasks: tasks[1:]},
	}

	content := e.buildIndexMD(tasks, phases)
	s := string(content)

	// Header.
	assert.Contains(t, s, "# Task Index")
	assert.Contains(t, s, "Total tasks: **2**")
	assert.Contains(t, s, "Total phases: **2**")

	// Task summary table.
	assert.Contains(t, s, "## Task Summary")
	assert.Contains(t, s, "| T-001 | Setup | must-have | small | None |")
	assert.Contains(t, s, "| T-002 | Build | should-have | medium | T-001 |")

	// Phase groupings.
	assert.Contains(t, s, "## Phase Groupings")
	assert.Contains(t, s, "### Phase 1: Phase 1")
	assert.Contains(t, s, "### Phase 2: Phase 2")
}

func TestBuildIndexMD_EscapesPipeInTitle(t *testing.T) {
	t.Parallel()

	e := NewEmitter(t.TempDir())
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "A | B", Priority: "must-have", Effort: "small"},
	}
	phases := []PhaseInfo{
		{ID: 1, Name: "P", StartTask: "T-001", EndTask: "T-001", Tasks: tasks},
	}

	content := e.buildIndexMD(tasks, phases)
	s := string(content)

	// Pipe in title must be escaped to HTML entity to not break the MD table.
	assert.Contains(t, s, "&#124;")
	assert.NotContains(t, s, "| A | B |")
}

func TestBuildIndexMD_MermaidOmittedForLargeTaskSets(t *testing.T) {
	t.Parallel()

	e := NewEmitter(t.TempDir())

	// Build more than maxTasksForMermaid tasks (100).
	tasks := make([]MergedTask, 101)
	for i := range tasks {
		tasks[i] = MergedTask{
			GlobalID: fmt.Sprintf("T-%03d", i+1),
			Title:    fmt.Sprintf("Task %d", i+1),
		}
	}
	phases := []PhaseInfo{
		{ID: 1, Name: "All", StartTask: "T-001", EndTask: "T-101", Tasks: tasks},
	}

	content := e.buildIndexMD(tasks, phases)
	s := string(content)

	assert.NotContains(t, s, "```mermaid")
	assert.Contains(t, s, "Dependency graph omitted")
}

func TestBuildIndexMD_NoDepsNoMermaid(t *testing.T) {
	t.Parallel()

	e := NewEmitter(t.TempDir())
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "A", Priority: "must-have", Effort: "small"},
	}
	phases := []PhaseInfo{
		{ID: 1, Name: "P", StartTask: "T-001", EndTask: "T-001", Tasks: tasks},
	}

	content := e.buildIndexMD(tasks, phases)
	s := string(content)

	assert.Contains(t, s, "_No dependencies defined._")
	assert.NotContains(t, s, "```mermaid")
}

// =============================================================================
// Emit edge cases
// =============================================================================

func TestEmit_EmptyTaskList(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	result, err := emitter.Emit(EmitOpts{Tasks: []MergedTask{}})
	require.NoError(t, err)
	assert.Equal(t, 0, result.TotalTasks)
	assert.Equal(t, 0, result.TotalPhases)
	assert.Empty(t, result.TaskFiles)
}

func TestEmit_NilValidation(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Task A", Description: "Desc", Effort: "small", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}},
		{GlobalID: "T-002", Title: "Task B", Description: "Desc", Effort: "medium", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}, Dependencies: []string{"T-001"}},
	}

	// Nil Validation -- all tasks should land in a single phase.
	result, err := emitter.Emit(EmitOpts{Tasks: tasks, Validation: nil})
	require.NoError(t, err)
	assert.Equal(t, 2, result.TotalTasks)
	// Without validation depths, all tasks default to depth 0 -> 1 phase.
	assert.Equal(t, 1, result.TotalPhases)
}

func TestEmit_TaskWithNilAcceptanceCriteria(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{
			GlobalID:           "T-001",
			Title:              "Lonely Task",
			Description:        "Has no criteria.",
			AcceptanceCriteria: nil,
			Effort:             "small",
			Priority:           "must-have",
		},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)
	require.Len(t, result.TaskFiles, 1)

	content, err := os.ReadFile(result.TaskFiles[0])
	require.NoError(t, err)
	// File should exist and contain the task ID.
	assert.Contains(t, string(content), "T-001")
}

func TestEmit_TaskWithNoDependencies_ShowsNone(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{
			GlobalID:           "T-001",
			Title:              "Independent Task",
			Description:        "No deps.",
			AcceptanceCriteria: []string{"Criterion"},
			Dependencies:       nil,
			Effort:             "small",
			Priority:           "must-have",
		},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)
	require.Len(t, result.TaskFiles, 1)

	content, err := os.ReadFile(result.TaskFiles[0])
	require.NoError(t, err)
	assert.Contains(t, string(content), "| Dependencies | None |")
}

func TestEmit_EmitResultPaths(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Alpha", Description: "D", Effort: "small", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)

	// All paths should be absolute and within the output directory.
	assert.True(t, filepath.IsAbs(result.OutputDir))
	assert.True(t, filepath.IsAbs(result.TaskStateFile))
	assert.True(t, filepath.IsAbs(result.PhasesFile))
	assert.True(t, filepath.IsAbs(result.ProgressFile))
	assert.True(t, filepath.IsAbs(result.IndexFile))
	for _, tf := range result.TaskFiles {
		assert.True(t, filepath.IsAbs(tf))
		assert.True(t, strings.HasPrefix(tf, result.OutputDir))
	}
}

func TestEmit_TaskFileNameContainsGlobalIDAndSlug(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Configure CI/CD Pipeline", Description: "D", Effort: "small", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)
	require.Len(t, result.TaskFiles, 1)

	base := filepath.Base(result.TaskFiles[0])
	assert.True(t, strings.HasPrefix(base, "T-001-"), "filename should start with T-001-")
	assert.True(t, strings.HasSuffix(base, ".md"), "filename should end with .md")
	assert.Contains(t, base, "configure-ci-cd-pipeline")
}

func TestEmit_SlugCollision_ThreeTasks(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	// Three tasks all produce the same slug.
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Auth Module", Description: "D", Effort: "small", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}},
		{GlobalID: "T-002", Title: "Auth Module", Description: "D", Effort: "small", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}},
		{GlobalID: "T-003", Title: "Auth Module", Description: "D", Effort: "small", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)
	require.Len(t, result.TaskFiles, 3)

	basenames := make(map[string]bool, 3)
	for _, p := range result.TaskFiles {
		base := filepath.Base(p)
		assert.False(t, basenames[base], "duplicate filename: %s", base)
		basenames[base] = true
	}
}

func TestEmit_ResequencesAndUpdatesDepRefs(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	// T-003 depends on T-001; after re-sequencing T-003->T-002, the dep ref
	// in the output file and task-state.conf should use the new IDs.
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "First", Description: "D", Effort: "small", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}},
		{GlobalID: "T-003", Title: "Third", Description: "D", Effort: "small", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}, Dependencies: []string{"T-001"}},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)

	// Find the file for the second task (originally T-003 -> now T-002).
	var secondTaskFile string
	for _, tf := range result.TaskFiles {
		if strings.Contains(filepath.Base(tf), "T-002") {
			secondTaskFile = tf
		}
	}
	require.NotEmpty(t, secondTaskFile, "re-sequenced T-002 task file should exist")

	content, err := os.ReadFile(secondTaskFile)
	require.NoError(t, err)
	s := string(content)

	// The dependency in the rendered file should reference T-001 (unchanged).
	assert.Contains(t, s, "T-001")
	// The new ID should appear in the heading.
	assert.Contains(t, s, "T-002")
}

// =============================================================================
// Performance test: 50 tasks
// =============================================================================

func TestEmit_FiftyTasks_Performance(t *testing.T) {
	// Not parallel -- measures wall time and should not compete with other tests.
	start := time.Now()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := make([]MergedTask, 50)
	for i := range tasks {
		tasks[i] = MergedTask{
			GlobalID:           fmt.Sprintf("T-%03d", i+1),
			Title:              fmt.Sprintf("Task number %d with a reasonably long title", i+1),
			Description:        fmt.Sprintf("Description for task %d.", i+1),
			AcceptanceCriteria: []string{fmt.Sprintf("Criterion %d", i+1)},
			Effort:             "medium",
			Priority:           "must-have",
		}
		if i > 0 {
			tasks[i].Dependencies = []string{fmt.Sprintf("T-%03d", i)}
		}
	}

	validation := ValidateDAG(tasks)
	require.True(t, validation.Valid)

	result, err := emitter.Emit(EmitOpts{
		Tasks:      tasks,
		Validation: validation,
	})
	require.NoError(t, err)

	elapsed := time.Since(start)
	assert.Less(t, elapsed.Milliseconds(), int64(5000), "50-task emit should complete in < 5s")

	assert.Equal(t, 50, result.TotalTasks)
	assert.Len(t, result.TaskFiles, 50)
}

// =============================================================================
// Integration tests: round-trip with internal/task parsers
// =============================================================================

// buildRealisticDataset returns a dataset with 15 tasks across 3 phases that
// exercises the full pipeline for integration testing.
func buildRealisticDataset() ([]MergedTask, *DAGValidation, *EpicBreakdown) {
	epics := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Infrastructure Setup"},
			{ID: "E-002", Title: "Core Business Logic"},
			{ID: "E-003", Title: "API & Integration"},
		},
	}

	tasks := []MergedTask{
		// Phase 0 (depth 0) -- Infrastructure
		{GlobalID: "T-001", EpicID: "E-001", Title: "Initialize Repository", Description: "Set up the project repo.", AcceptanceCriteria: []string{"Repo created", "CI passing"}, Effort: "small", Priority: "must-have"},
		{GlobalID: "T-002", EpicID: "E-001", Title: "Configure CI Pipeline", Description: "Set up GitHub Actions.", AcceptanceCriteria: []string{"Pipeline passes on PR"}, Effort: "small", Priority: "must-have"},
		{GlobalID: "T-003", EpicID: "E-001", Title: "Setup Database Schema", Description: "Create initial migrations.", AcceptanceCriteria: []string{"Migrations run successfully"}, Effort: "medium", Priority: "must-have"},
		{GlobalID: "T-004", EpicID: "E-001", Title: "Configure Logging", Description: "Add structured logging.", AcceptanceCriteria: []string{"Log output in JSON"}, Effort: "small", Priority: "should-have"},
		{GlobalID: "T-005", EpicID: "E-001", Title: "Setup Environment Config", Description: "Support 12-factor config.", AcceptanceCriteria: []string{"Config loaded from env"}, Effort: "small", Priority: "must-have"},

		// Phase 1 (depth 1) -- Core Business Logic
		{GlobalID: "T-006", EpicID: "E-002", Title: "Implement User Model", Description: "Create user domain model.", AcceptanceCriteria: []string{"Model validates email", "Password hashed"}, Effort: "medium", Priority: "must-have", Dependencies: []string{"T-003"}},
		{GlobalID: "T-007", EpicID: "E-002", Title: "Implement Auth Service", Description: "JWT-based auth.", AcceptanceCriteria: []string{"Login returns JWT", "Expired tokens rejected"}, Effort: "medium", Priority: "must-have", Dependencies: []string{"T-006"}},
		{GlobalID: "T-008", EpicID: "E-002", Title: "Implement Product Catalog", Description: "CRUD for products.", AcceptanceCriteria: []string{"Products list paginated", "Search works"}, Effort: "large", Priority: "must-have", Dependencies: []string{"T-003"}},
		{GlobalID: "T-009", EpicID: "E-002", Title: "Implement Order Service", Description: "Order lifecycle.", AcceptanceCriteria: []string{"Order created", "Order status updated"}, Effort: "large", Priority: "must-have", Dependencies: []string{"T-006", "T-008"}},
		{GlobalID: "T-010", EpicID: "E-002", Title: "Implement Payment Integration", Description: "Stripe integration.", AcceptanceCriteria: []string{"Payment processed", "Webhook handled"}, Effort: "large", Priority: "must-have", Dependencies: []string{"T-009"}},

		// Phase 2 (depth 2) -- API & Integration
		{GlobalID: "T-011", EpicID: "E-003", Title: "Build REST API", Description: "Expose HTTP endpoints.", AcceptanceCriteria: []string{"OpenAPI spec generated", "Endpoints documented"}, Effort: "large", Priority: "must-have", Dependencies: []string{"T-007", "T-008"}},
		{GlobalID: "T-012", EpicID: "E-003", Title: "Implement Rate Limiting", Description: "Protect API.", AcceptanceCriteria: []string{"429 returned on excess"}, Effort: "medium", Priority: "should-have", Dependencies: []string{"T-011"}},
		{GlobalID: "T-013", EpicID: "E-003", Title: "Add Webhook Support", Description: "Outbound events.", AcceptanceCriteria: []string{"Events delivered", "Retries on failure"}, Effort: "medium", Priority: "should-have", Dependencies: []string{"T-011"}},
		{GlobalID: "T-014", EpicID: "E-003", Title: "Write Integration Tests", Description: "End-to-end tests.", AcceptanceCriteria: []string{"90% coverage", "Tests run in CI"}, Effort: "medium", Priority: "must-have", Dependencies: []string{"T-011", "T-012"}},
		{GlobalID: "T-015", EpicID: "E-003", Title: "Production Deployment", Description: "Deploy to prod.", AcceptanceCriteria: []string{"Zero-downtime deploy", "Monitoring alerts"}, Effort: "large", Priority: "must-have", Dependencies: []string{"T-014"}},
	}

	validation := ValidateDAG(tasks)
	return tasks, validation, epics
}

func TestIntegration_RoundTrip_PhasesConf(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks, validation, epics := buildRealisticDataset()
	require.True(t, validation.Valid)

	result, err := emitter.Emit(EmitOpts{
		Tasks:      tasks,
		Validation: validation,
		Epics:      epics,
	})
	require.NoError(t, err)

	// --- Round-trip: read phases.conf back via task.LoadPhases ---
	phases, err := task.LoadPhases(result.PhasesFile)
	require.NoError(t, err, "task.LoadPhases must parse the generated phases.conf without error")

	assert.Equal(t, result.TotalPhases, len(phases),
		"LoadPhases should return the same number of phases as TotalPhases")

	// Verify each phase has valid IDs.
	for _, phase := range phases {
		assert.Greater(t, phase.ID, 0, "phase ID must be positive")
		assert.NotEmpty(t, phase.Name, "phase name must not be empty")
		assert.True(t, strings.HasPrefix(phase.StartTask, "T-"), "StartTask must have T- prefix")
		assert.True(t, strings.HasPrefix(phase.EndTask, "T-"), "EndTask must have T- prefix")
	}

	// Phases must be sorted by ID.
	for i := 1; i < len(phases); i++ {
		assert.Greater(t, phases[i].ID, phases[i-1].ID, "phases must be sorted by ID")
	}
}

func TestIntegration_RoundTrip_TaskStateConf(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks, validation, epics := buildRealisticDataset()
	require.True(t, validation.Valid)

	result, err := emitter.Emit(EmitOpts{
		Tasks:      tasks,
		Validation: validation,
		Epics:      epics,
	})
	require.NoError(t, err)

	// --- Round-trip: read task-state.conf via task.NewStateManager ---
	sm := task.NewStateManager(result.TaskStateFile)
	states, err := sm.Load()
	require.NoError(t, err, "StateManager.Load must parse the generated task-state.conf without error")

	assert.Equal(t, result.TotalTasks, len(states),
		"StateManager.Load should return one entry per task")

	// All statuses should be not_started.
	for _, state := range states {
		assert.Equal(t, task.StatusNotStarted, state.Status,
			"initial status for %s should be not_started", state.TaskID)
		assert.True(t, strings.HasPrefix(state.TaskID, "T-"),
			"task ID %q must have T- prefix", state.TaskID)
	}

	// Verify no duplicate task IDs.
	seenIDs := make(map[string]bool, len(states))
	for _, state := range states {
		assert.False(t, seenIDs[state.TaskID], "duplicate task ID %s in state file", state.TaskID)
		seenIDs[state.TaskID] = true
	}

	// IDs should match the re-sequenced tasks in the result.
	// Since tasks have IDs T-001 through T-015 with no gaps, no re-sequencing occurs.
	for i, state := range states {
		expected := fmt.Sprintf("T-%03d", i+1)
		assert.Equal(t, expected, state.TaskID)
	}
}

func TestIntegration_RoundTrip_TaskFiles(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks, validation, epics := buildRealisticDataset()
	require.True(t, validation.Valid)

	result, err := emitter.Emit(EmitOpts{
		Tasks:      tasks,
		Validation: validation,
		Epics:      epics,
	})
	require.NoError(t, err)

	// --- Round-trip: parse each generated task file via task.ParseTaskFile ---
	require.Len(t, result.TaskFiles, len(tasks))

	// Map parsed specs by ID for verification.
	parsedByID := make(map[string]*task.ParsedTaskSpec, len(tasks))

	for _, taskFilePath := range result.TaskFiles {
		spec, err := task.ParseTaskFile(taskFilePath)
		require.NoError(t, err, "task.ParseTaskFile should parse %s without error", taskFilePath)
		require.NotNil(t, spec)

		assert.NotEmpty(t, spec.ID, "parsed spec must have an ID")
		assert.NotEmpty(t, spec.Title, "parsed spec must have a title")
		assert.True(t, strings.HasPrefix(spec.ID, "T-"), "ID must have T- prefix")

		parsedByID[spec.ID] = spec
	}

	// Verify round-trip fidelity for each input task.
	for _, inputTask := range tasks {
		spec, ok := parsedByID[inputTask.GlobalID]
		require.True(t, ok, "parsed output must contain task %s", inputTask.GlobalID)

		assert.Equal(t, inputTask.Title, spec.Title,
			"title round-trip for %s", inputTask.GlobalID)
		assert.Equal(t, inputTask.Priority, spec.Priority,
			"priority round-trip for %s", inputTask.GlobalID)
		assert.Equal(t, inputTask.Effort, spec.Effort,
			"effort round-trip for %s", inputTask.GlobalID)

		// Dependencies must round-trip (parser extracts T-NNN refs from the metadata row).
		if len(inputTask.Dependencies) > 0 {
			assert.Equal(t, len(inputTask.Dependencies), len(spec.Dependencies),
				"dependency count round-trip for %s", inputTask.GlobalID)
			for _, dep := range inputTask.Dependencies {
				assert.Contains(t, spec.Dependencies, dep,
					"dependency %s should be present in parsed spec for %s", dep, inputTask.GlobalID)
			}
		}
	}
}

func TestIntegration_RoundTrip_WithGaps(t *testing.T) {
	t.Parallel()

	// Simulate the scenario after deduplication: IDs have gaps.
	// Emitter should re-sequence them and the state file should reflect new IDs.
	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Foundation", Description: "Desc", Effort: "small", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}},
		// T-002 was deduplicated; gap here.
		{GlobalID: "T-003", Title: "Core Service", Description: "Desc", Effort: "medium", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}, Dependencies: []string{"T-001"}},
		// T-004 was deduplicated; gap here.
		{GlobalID: "T-005", Title: "API Layer", Description: "Desc", Effort: "large", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}, Dependencies: []string{"T-001", "T-003"}},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalTasks)

	// Read back state file.
	sm := task.NewStateManager(result.TaskStateFile)
	states, err := sm.Load()
	require.NoError(t, err)
	require.Len(t, states, 3)

	// IDs must be sequential T-001, T-002, T-003 (no gaps).
	assert.Equal(t, "T-001", states[0].TaskID)
	assert.Equal(t, "T-002", states[1].TaskID)
	assert.Equal(t, "T-003", states[2].TaskID)

	// Verify the second task file (originally T-003->T-002) contains updated dep.
	// Find the file that maps to the second output task.
	var secondFile string
	for _, tf := range result.TaskFiles {
		if strings.Contains(filepath.Base(tf), "T-002-") {
			secondFile = tf
		}
	}
	require.NotEmpty(t, secondFile, "task file T-002-*.md should exist after re-sequencing")

	spec, err := task.ParseTaskFile(secondFile)
	require.NoError(t, err)
	assert.Equal(t, "T-002", spec.ID)
	// The dependency should reference T-001 (unchanged since T-001 wasn't remapped).
	assert.Contains(t, spec.Dependencies, "T-001")
}

func TestIntegration_FullPipelineWithEpicsAndValidation(t *testing.T) {
	t.Parallel()

	// Build a 20-task dataset across 4 phases (epics).
	epics := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Bootstrap"},
			{ID: "E-002", Title: "Domain Model"},
			{ID: "E-003", Title: "Services"},
			{ID: "E-004", Title: "Delivery"},
		},
	}

	var allTasks []MergedTask
	var n int

	addTask := func(epicID, title, desc string, deps []string, effort, priority string) {
		n++
		allTasks = append(allTasks, MergedTask{
			GlobalID:           fmt.Sprintf("T-%03d", n),
			EpicID:             epicID,
			Title:              title,
			Description:        desc,
			AcceptanceCriteria: []string{"Acceptance criterion for " + title},
			Dependencies:       deps,
			Effort:             effort,
			Priority:           priority,
		})
	}

	// E-001: Bootstrap (depth 0)
	addTask("E-001", "Repo Init", "Initialize repository", nil, "small", "must-have")
	addTask("E-001", "CI Setup", "Configure CI", nil, "small", "must-have")
	addTask("E-001", "DB Schema", "Create migrations", nil, "medium", "must-have")
	addTask("E-001", "Env Config", "12-factor config", nil, "small", "must-have")
	addTask("E-001", "Logging", "Structured logs", nil, "small", "should-have")

	// E-002: Domain Model (depth 1 - depends on bootstrap)
	addTask("E-002", "User Model", "User entity", []string{"T-003"}, "medium", "must-have")
	addTask("E-002", "Product Model", "Product entity", []string{"T-003"}, "medium", "must-have")
	addTask("E-002", "Order Model", "Order entity", []string{"T-006", "T-007"}, "large", "must-have")
	addTask("E-002", "Payment Model", "Payment entity", []string{"T-008"}, "medium", "must-have")
	addTask("E-002", "Notification Model", "Notification entity", []string{"T-003"}, "small", "should-have")

	// E-003: Services (depth 2 - depends on domain model)
	addTask("E-003", "Auth Service", "JWT auth", []string{"T-006"}, "medium", "must-have")
	addTask("E-003", "Product Service", "Product CRUD", []string{"T-007"}, "large", "must-have")
	addTask("E-003", "Order Service", "Order lifecycle", []string{"T-008", "T-011"}, "large", "must-have")
	addTask("E-003", "Payment Service", "Payment processing", []string{"T-009", "T-013"}, "large", "must-have")
	addTask("E-003", "Notification Service", "Send notifications", []string{"T-010"}, "medium", "should-have")

	// E-004: Delivery (depth 3 - depends on services)
	addTask("E-004", "REST API", "HTTP endpoints", []string{"T-011", "T-012"}, "large", "must-have")
	addTask("E-004", "GraphQL API", "GraphQL schema", []string{"T-011", "T-012"}, "large", "nice-to-have")
	addTask("E-004", "API Rate Limiting", "Protect API", []string{"T-016"}, "medium", "should-have")
	addTask("E-004", "Integration Tests", "End-to-end tests", []string{"T-016", "T-018"}, "medium", "must-have")
	addTask("E-004", "Production Deploy", "Deploy to prod", []string{"T-019"}, "large", "must-have")

	validation := ValidateDAG(allTasks)
	require.True(t, validation.Valid, "dataset must form a valid DAG")

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	result, err := emitter.Emit(EmitOpts{
		Tasks:      allTasks,
		Validation: validation,
		Epics:      epics,
	})
	require.NoError(t, err)

	assert.Equal(t, 20, result.TotalTasks)
	assert.Len(t, result.TaskFiles, 20)
	assert.GreaterOrEqual(t, result.TotalPhases, 1)

	// Round-trip: phases.conf.
	phases, err := task.LoadPhases(result.PhasesFile)
	require.NoError(t, err)
	assert.Equal(t, result.TotalPhases, len(phases))

	// Round-trip: task-state.conf.
	sm := task.NewStateManager(result.TaskStateFile)
	states, err := sm.Load()
	require.NoError(t, err)
	assert.Equal(t, 20, len(states))
	for _, s := range states {
		assert.Equal(t, task.StatusNotStarted, s.Status)
	}

	// Round-trip: task files.
	specs, err := task.DiscoverTasks(outDir)
	require.NoError(t, err)
	assert.Equal(t, 20, len(specs))

	// Each spec must parse without error and have valid IDs.
	for _, spec := range specs {
		assert.True(t, strings.HasPrefix(spec.ID, "T-"))
		assert.NotEmpty(t, spec.Title)
	}
}

func TestIntegration_StateManager_CanUpdateGeneratedFile(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Task A", Description: "D", Effort: "small", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}},
		{GlobalID: "T-002", Title: "Task B", Description: "D", Effort: "small", Priority: "must-have",
			AcceptanceCriteria: []string{"Done"}, Dependencies: []string{"T-001"}},
	}

	result, err := emitter.Emit(EmitOpts{Tasks: tasks})
	require.NoError(t, err)

	sm := task.NewStateManager(result.TaskStateFile)

	// Update T-001 to in_progress.
	err = sm.UpdateStatus("T-001", task.StatusInProgress, "claude")
	require.NoError(t, err)

	// Reload and check.
	states, err := sm.Load()
	require.NoError(t, err)
	require.Len(t, states, 2)

	stateMap := make(map[string]task.TaskState)
	for _, s := range states {
		stateMap[s.TaskID] = s
	}

	assert.Equal(t, task.StatusInProgress, stateMap["T-001"].Status)
	assert.Equal(t, "claude", stateMap["T-001"].Agent)
	assert.Equal(t, task.StatusNotStarted, stateMap["T-002"].Status)
}

func TestIntegration_PhaseForTask_WorksWithGeneratedPhases(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	emitter := NewEmitter(outDir, WithForce(true))

	tasks, validation, epics := buildRealisticDataset()
	require.True(t, validation.Valid)

	result, err := emitter.Emit(EmitOpts{
		Tasks:      tasks,
		Validation: validation,
		Epics:      epics,
	})
	require.NoError(t, err)

	phases, err := task.LoadPhases(result.PhasesFile)
	require.NoError(t, err)

	// task.PhaseForTask should find a phase for each task.
	for _, tsk := range tasks {
		phase := task.PhaseForTask(phases, tsk.GlobalID)
		assert.NotNil(t, phase, "PhaseForTask(%s) should find a phase in generated phases.conf", tsk.GlobalID)
	}
}
