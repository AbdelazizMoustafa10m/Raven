package prd

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- SortEpicsByDependency tests ---

func TestSortEpicsByDependency_SingleEpicNoDeps(t *testing.T) {
	t.Parallel()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Only", Description: "The only epic"},
		},
	}

	order, err := SortEpicsByDependency(breakdown)
	require.NoError(t, err)
	assert.Equal(t, []string{"E-001"}, order)
}

func TestSortEpicsByDependency_NilBreakdown(t *testing.T) {
	t.Parallel()

	order, err := SortEpicsByDependency(nil)
	require.NoError(t, err)
	assert.Empty(t, order)
}

func TestSortEpicsByDependency_EmptyEpics(t *testing.T) {
	t.Parallel()

	order, err := SortEpicsByDependency(&EpicBreakdown{Epics: []Epic{}})
	require.NoError(t, err)
	assert.Empty(t, order)
}

func TestSortEpicsByDependency_LinearChain(t *testing.T) {
	t.Parallel()

	// E-001 <- E-002 <- E-003 (each depends on the prior)
	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "First", Description: "No deps"},
			{ID: "E-002", Title: "Second", Description: "Depends on E-001", DependenciesOnEpics: []string{"E-001"}},
			{ID: "E-003", Title: "Third", Description: "Depends on E-002", DependenciesOnEpics: []string{"E-002"}},
		},
	}

	order, err := SortEpicsByDependency(breakdown)
	require.NoError(t, err)
	assert.Equal(t, []string{"E-001", "E-002", "E-003"}, order)
}

func TestSortEpicsByDependency_MultipleRoots(t *testing.T) {
	t.Parallel()

	// E-001 and E-002 have no deps; E-003 depends on both
	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D"},
			{ID: "E-002", Title: "B", Description: "D"},
			{ID: "E-003", Title: "C", Description: "D", DependenciesOnEpics: []string{"E-001", "E-002"}},
		},
	}

	order, err := SortEpicsByDependency(breakdown)
	require.NoError(t, err)
	require.Len(t, order, 3)
	// E-001 and E-002 come before E-003
	assert.Equal(t, "E-003", order[2])
	// First two should be E-001 and E-002 in sorted order
	assert.Equal(t, "E-001", order[0])
	assert.Equal(t, "E-002", order[1])
}

func TestSortEpicsByDependency_Deterministic_SortedOrder(t *testing.T) {
	t.Parallel()

	// All three epics are roots (no deps) — they should come out sorted by ID.
	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-003", Title: "C", Description: "D"},
			{ID: "E-001", Title: "A", Description: "D"},
			{ID: "E-002", Title: "B", Description: "D"},
		},
	}

	order, err := SortEpicsByDependency(breakdown)
	require.NoError(t, err)
	assert.Equal(t, []string{"E-001", "E-002", "E-003"}, order)
}

func TestSortEpicsByDependency_CycleDetected(t *testing.T) {
	t.Parallel()

	// E-002 depends on E-003, E-003 depends on E-002 — cycle
	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D"},
			{ID: "E-002", Title: "B", Description: "D", DependenciesOnEpics: []string{"E-003"}},
			{ID: "E-003", Title: "C", Description: "D", DependenciesOnEpics: []string{"E-002"}},
		},
	}

	order, err := SortEpicsByDependency(breakdown)
	require.Error(t, err)
	assert.Nil(t, order)
	assert.Contains(t, err.Error(), "cyclic epic dependency detected")
	assert.Contains(t, err.Error(), "E-002")
	assert.Contains(t, err.Error(), "E-003")
}

func TestSortEpicsByDependency_CycleErrorMessage_Informative(t *testing.T) {
	t.Parallel()

	// Three-node cycle: E-001 -> E-002 -> E-003 -> E-001
	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D", DependenciesOnEpics: []string{"E-003"}},
			{ID: "E-002", Title: "B", Description: "D", DependenciesOnEpics: []string{"E-001"}},
			{ID: "E-003", Title: "C", Description: "D", DependenciesOnEpics: []string{"E-002"}},
		},
	}

	_, err := SortEpicsByDependency(breakdown)
	require.Error(t, err)
	// The error must mention "form a cycle" and include the involved epic IDs.
	assert.Contains(t, err.Error(), "form a cycle")
	assert.Contains(t, err.Error(), "E-001")
	assert.Contains(t, err.Error(), "E-002")
	assert.Contains(t, err.Error(), "E-003")
}

func TestSortEpicsByDependency_PartialCycle_RootEpicsOrdered(t *testing.T) {
	t.Parallel()

	// E-001 is a root; E-002 and E-003 form a cycle.
	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Root", Description: "D"},
			{ID: "E-002", Title: "CycleA", Description: "D", DependenciesOnEpics: []string{"E-003"}},
			{ID: "E-003", Title: "CycleB", Description: "D", DependenciesOnEpics: []string{"E-002"}},
		},
	}

	_, err := SortEpicsByDependency(breakdown)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cyclic epic dependency detected")
}

// --- AssignGlobalIDs tests ---

func TestAssignGlobalIDs_SingleEpic_ThreeTasks(t *testing.T) {
	t.Parallel()

	order := []string{"E-001"}
	results := map[string]*EpicTaskResult{
		"E-001": {
			EpicID: "E-001",
			Tasks: []TaskDef{
				{TempID: "E001-T01", Title: "First task", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
				{TempID: "E001-T02", Title: "Second task", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "medium", Priority: "should-have"},
				{TempID: "E001-T03", Title: "Third task", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "large", Priority: "nice-to-have"},
			},
		},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	require.Len(t, merged, 3)
	assert.Equal(t, "T-001", merged[0].GlobalID)
	assert.Equal(t, "E001-T01", merged[0].TempID)
	assert.Equal(t, "E-001", merged[0].EpicID)

	assert.Equal(t, "T-002", merged[1].GlobalID)
	assert.Equal(t, "E001-T02", merged[1].TempID)

	assert.Equal(t, "T-003", merged[2].GlobalID)
	assert.Equal(t, "E001-T03", merged[2].TempID)

	// Verify mapping
	assert.Equal(t, "T-001", mapping["E001-T01"])
	assert.Equal(t, "T-002", mapping["E001-T02"])
	assert.Equal(t, "T-003", mapping["E001-T03"])
}

func TestAssignGlobalIDs_LinearChain_EpicOrder(t *testing.T) {
	t.Parallel()

	// E-001 has 2 tasks, E-002 has 3 tasks, E-003 has 1 task.
	order := []string{"E-001", "E-002", "E-003"}
	results := map[string]*EpicTaskResult{
		"E-001": {
			EpicID: "E-001",
			Tasks: []TaskDef{
				{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
				{TempID: "E001-T02", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			},
		},
		"E-002": {
			EpicID: "E-002",
			Tasks: []TaskDef{
				{TempID: "E002-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
				{TempID: "E002-T02", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
				{TempID: "E002-T03", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			},
		},
		"E-003": {
			EpicID: "E-003",
			Tasks: []TaskDef{
				{TempID: "E003-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			},
		},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	require.Len(t, merged, 6)

	// E-001 tasks get T-001, T-002
	assert.Equal(t, "T-001", merged[0].GlobalID)
	assert.Equal(t, "E001-T01", merged[0].TempID)
	assert.Equal(t, "E-001", merged[0].EpicID)

	assert.Equal(t, "T-002", merged[1].GlobalID)
	assert.Equal(t, "E001-T02", merged[1].TempID)

	// E-002 tasks get T-003, T-004, T-005
	assert.Equal(t, "T-003", merged[2].GlobalID)
	assert.Equal(t, "E002-T01", merged[2].TempID)
	assert.Equal(t, "E-002", merged[2].EpicID)

	assert.Equal(t, "T-004", merged[3].GlobalID)
	assert.Equal(t, "T-005", merged[4].GlobalID)

	// E-003 task gets T-006
	assert.Equal(t, "T-006", merged[5].GlobalID)
	assert.Equal(t, "E003-T01", merged[5].TempID)
	assert.Equal(t, "E-003", merged[5].EpicID)

	// Verify IDMapping completeness
	assert.Len(t, mapping, 6)
	assert.Equal(t, "T-001", mapping["E001-T01"])
	assert.Equal(t, "T-003", mapping["E002-T01"])
	assert.Equal(t, "T-006", mapping["E003-T01"])
}

func TestAssignGlobalIDs_EpicWithNoTasks_SkippedNoGap(t *testing.T) {
	t.Parallel()

	order := []string{"E-001", "E-002", "E-003"}
	results := map[string]*EpicTaskResult{
		"E-001": {
			EpicID: "E-001",
			Tasks: []TaskDef{
				{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			},
		},
		"E-002": {
			EpicID: "E-002",
			Tasks:  []TaskDef{}, // zero tasks
		},
		"E-003": {
			EpicID: "E-003",
			Tasks: []TaskDef{
				{TempID: "E003-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			},
		},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	// E-002 has no tasks, so total is 2; no gap in numbering.
	require.Len(t, merged, 2)
	assert.Equal(t, "T-001", merged[0].GlobalID)
	assert.Equal(t, "E001-T01", merged[0].TempID)

	assert.Equal(t, "T-002", merged[1].GlobalID)
	assert.Equal(t, "E003-T01", merged[1].TempID)

	assert.Len(t, mapping, 2)
}

func TestAssignGlobalIDs_EpicInResultsNotInOrder_AppendedSorted(t *testing.T) {
	t.Parallel()

	// epicOrder only includes E-001; E-002 is extra (not in order)
	order := []string{"E-001"}
	results := map[string]*EpicTaskResult{
		"E-001": {
			EpicID: "E-001",
			Tasks: []TaskDef{
				{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			},
		},
		"E-002": {
			EpicID: "E-002",
			Tasks: []TaskDef{
				{TempID: "E002-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			},
		},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	require.Len(t, merged, 2)
	// E-001 tasks first (in epicOrder), then E-002 appended.
	assert.Equal(t, "T-001", merged[0].GlobalID)
	assert.Equal(t, "E-001", merged[0].EpicID)

	assert.Equal(t, "T-002", merged[1].GlobalID)
	assert.Equal(t, "E-002", merged[1].EpicID)

	assert.Equal(t, "T-001", mapping["E001-T01"])
	assert.Equal(t, "T-002", mapping["E002-T01"])
}

func TestAssignGlobalIDs_EpicInOrderNotInResults_Skipped(t *testing.T) {
	t.Parallel()

	// E-002 is in epicOrder but not in results
	order := []string{"E-001", "E-002", "E-003"}
	results := map[string]*EpicTaskResult{
		"E-001": {
			EpicID: "E-001",
			Tasks: []TaskDef{
				{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			},
		},
		"E-003": {
			EpicID: "E-003",
			Tasks: []TaskDef{
				{TempID: "E003-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			},
		},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	require.Len(t, merged, 2)
	assert.Equal(t, "T-001", merged[0].GlobalID)
	assert.Equal(t, "E001-T01", merged[0].TempID)

	// E-002 skipped; E-003 gets T-002 with no gap.
	assert.Equal(t, "T-002", merged[1].GlobalID)
	assert.Equal(t, "E003-T01", merged[1].TempID)

	assert.Len(t, mapping, 2)
}

func TestAssignGlobalIDs_EmptyResults(t *testing.T) {
	t.Parallel()

	merged, mapping := AssignGlobalIDs([]string{}, map[string]*EpicTaskResult{})

	assert.Empty(t, merged)
	assert.Empty(t, mapping)
}

func TestAssignGlobalIDs_NilResults(t *testing.T) {
	t.Parallel()

	merged, mapping := AssignGlobalIDs(nil, map[string]*EpicTaskResult{})

	assert.Empty(t, merged)
	assert.Empty(t, mapping)
}

func TestAssignGlobalIDs_FourDigitPadding_1000Tasks(t *testing.T) {
	t.Parallel()

	// Build exactly 1000 tasks across two epics to trigger 4-digit padding.
	tasks1 := make([]TaskDef, 500)
	for i := range tasks1 {
		tasks1[i] = TaskDef{
			TempID:             fmt.Sprintf("E001-T%02d-%03d", i/100+1, i),
			Title:              "T",
			Description:        "D",
			AcceptanceCriteria: []string{"ac"},
			Effort:             "small",
			Priority:           "must-have",
		}
	}
	tasks2 := make([]TaskDef, 500)
	for i := range tasks2 {
		tasks2[i] = TaskDef{
			TempID:             fmt.Sprintf("E002-T%02d-%03d", i/100+1, i),
			Title:              "T",
			Description:        "D",
			AcceptanceCriteria: []string{"ac"},
			Effort:             "small",
			Priority:           "must-have",
		}
	}

	order := []string{"E-001", "E-002"}
	results := map[string]*EpicTaskResult{
		"E-001": {EpicID: "E-001", Tasks: tasks1},
		"E-002": {EpicID: "E-002", Tasks: tasks2},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	require.Len(t, merged, 1000)
	// First task should be T-0001 (4-digit padding).
	assert.Equal(t, "T-0001", merged[0].GlobalID)
	// Last task should be T-1000.
	assert.Equal(t, "T-1000", merged[999].GlobalID)

	// All 1000 tasks should be in the mapping.
	assert.Len(t, mapping, 1000)
}

func TestAssignGlobalIDs_999Tasks_ThreeDigitPadding(t *testing.T) {
	t.Parallel()

	tasks := make([]TaskDef, 999)
	for i := range tasks {
		tasks[i] = TaskDef{
			TempID:             fmt.Sprintf("E001-T%02d-%03d", i/100+1, i),
			Title:              "T",
			Description:        "D",
			AcceptanceCriteria: []string{"ac"},
			Effort:             "small",
			Priority:           "must-have",
		}
	}

	order := []string{"E-001"}
	results := map[string]*EpicTaskResult{
		"E-001": {EpicID: "E-001", Tasks: tasks},
	}

	merged, _ := AssignGlobalIDs(order, results)

	require.Len(t, merged, 999)
	assert.Equal(t, "T-001", merged[0].GlobalID)
	assert.Equal(t, "T-999", merged[998].GlobalID)
}

func TestAssignGlobalIDs_TaskFieldsPreserved(t *testing.T) {
	t.Parallel()

	order := []string{"E-001"}
	results := map[string]*EpicTaskResult{
		"E-001": {
			EpicID: "E-001",
			Tasks: []TaskDef{
				{
					TempID:             "E001-T01",
					Title:              "Implement auth middleware",
					Description:        "Auth middleware implementation",
					AcceptanceCriteria: []string{"Tokens validated", "Expired tokens rejected"},
					LocalDependencies:  []string{"E001-T00"},
					CrossEpicDeps:      []string{"E-002:db-schema"},
					Effort:             "medium",
					Priority:           "must-have",
				},
			},
		},
	}

	merged, _ := AssignGlobalIDs(order, results)

	require.Len(t, merged, 1)
	mt := merged[0]
	assert.Equal(t, "T-001", mt.GlobalID)
	assert.Equal(t, "E001-T01", mt.TempID)
	assert.Equal(t, "E-001", mt.EpicID)
	assert.Equal(t, "Implement auth middleware", mt.Title)
	assert.Equal(t, "Auth middleware implementation", mt.Description)
	assert.Equal(t, []string{"Tokens validated", "Expired tokens rejected"}, mt.AcceptanceCriteria)
	assert.Equal(t, []string{"E001-T00"}, mt.LocalDependencies)
	assert.Equal(t, []string{"E-002:db-schema"}, mt.CrossEpicDeps)
	assert.Equal(t, "medium", mt.Effort)
	assert.Equal(t, "must-have", mt.Priority)
}

// --- Integration: SortEpicsByDependency + AssignGlobalIDs ---

func TestSortAndAssign_LinearChain(t *testing.T) {
	t.Parallel()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "First", Description: "No deps"},
			{ID: "E-002", Title: "Second", Description: "Depends on E-001", DependenciesOnEpics: []string{"E-001"}},
			{ID: "E-003", Title: "Third", Description: "Depends on E-002", DependenciesOnEpics: []string{"E-002"}},
		},
	}

	order, err := SortEpicsByDependency(breakdown)
	require.NoError(t, err)
	require.Equal(t, []string{"E-001", "E-002", "E-003"}, order)

	results := map[string]*EpicTaskResult{
		"E-001": {EpicID: "E-001", Tasks: []TaskDef{
			{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			{TempID: "E001-T02", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
		"E-002": {EpicID: "E-002", Tasks: []TaskDef{
			{TempID: "E002-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
		"E-003": {EpicID: "E-003", Tasks: []TaskDef{
			{TempID: "E003-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			{TempID: "E003-T02", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	require.Len(t, merged, 5)
	assert.Equal(t, "T-001", merged[0].GlobalID)
	assert.Equal(t, "T-002", merged[1].GlobalID)
	assert.Equal(t, "T-003", merged[2].GlobalID)
	assert.Equal(t, "T-004", merged[3].GlobalID)
	assert.Equal(t, "T-005", merged[4].GlobalID)

	assert.Equal(t, "E-001", merged[0].EpicID)
	assert.Equal(t, "E-001", merged[1].EpicID)
	assert.Equal(t, "E-002", merged[2].EpicID)
	assert.Equal(t, "E-003", merged[3].EpicID)
	assert.Equal(t, "E-003", merged[4].EpicID)

	assert.Len(t, mapping, 5)
	assert.Equal(t, "T-001", mapping["E001-T01"])
	assert.Equal(t, "T-002", mapping["E001-T02"])
	assert.Equal(t, "T-003", mapping["E002-T01"])
	assert.Equal(t, "T-004", mapping["E003-T01"])
	assert.Equal(t, "T-005", mapping["E003-T02"])
}

func TestSortAndAssign_CycleError(t *testing.T) {
	t.Parallel()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D", DependenciesOnEpics: []string{"E-002"}},
			{ID: "E-002", Title: "B", Description: "D", DependenciesOnEpics: []string{"E-001"}},
		},
	}

	_, err := SortEpicsByDependency(breakdown)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cyclic epic dependency detected")
}

func TestAssignGlobalIDs_MultipleExtrasAppendedSorted(t *testing.T) {
	t.Parallel()

	// epicOrder is empty; all epics are extras — should be sorted by ID.
	order := []string{}
	results := map[string]*EpicTaskResult{
		"E-003": {EpicID: "E-003", Tasks: []TaskDef{
			{TempID: "E003-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
		"E-001": {EpicID: "E-001", Tasks: []TaskDef{
			{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
		"E-002": {EpicID: "E-002", Tasks: []TaskDef{
			{TempID: "E002-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	require.Len(t, merged, 3)
	// All epics are extras; sorted by ID => E-001, E-002, E-003.
	assert.Equal(t, "E-001", merged[0].EpicID)
	assert.Equal(t, "T-001", merged[0].GlobalID)

	assert.Equal(t, "E-002", merged[1].EpicID)
	assert.Equal(t, "T-002", merged[1].GlobalID)

	assert.Equal(t, "E-003", merged[2].EpicID)
	assert.Equal(t, "T-003", merged[2].GlobalID)

	assert.Len(t, mapping, 3)
}

// --- Additional tests for T-060 ---

// TestSortEpicsByDependency_DiamondDependency verifies the four-epic diamond shape:
// E-001 has no deps; E-002 and E-003 both depend on E-001; E-004 depends on E-002 and E-003.
func TestSortEpicsByDependency_DiamondDependency(t *testing.T) {
	t.Parallel()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Root", Description: "No deps"},
			{ID: "E-002", Title: "Left", Description: "Depends on E-001", DependenciesOnEpics: []string{"E-001"}},
			{ID: "E-003", Title: "Right", Description: "Depends on E-001", DependenciesOnEpics: []string{"E-001"}},
			{ID: "E-004", Title: "Sink", Description: "Depends on E-002 and E-003", DependenciesOnEpics: []string{"E-002", "E-003"}},
		},
	}

	order, err := SortEpicsByDependency(breakdown)
	require.NoError(t, err)
	require.Len(t, order, 4)

	// E-001 must be first.
	assert.Equal(t, "E-001", order[0])

	// E-002 and E-003 must both appear before E-004; their relative order is determined
	// lexicographically (E-002 before E-003).
	assert.Equal(t, "E-002", order[1])
	assert.Equal(t, "E-003", order[2])

	// E-004 must be last (depends on E-002 and E-003).
	assert.Equal(t, "E-004", order[3])
}

// TestSortEpicsByDependency_DiamondDependency_IDMapping verifies that IDs are assigned
// sequentially after topological sort of a diamond dependency graph.
func TestSortEpicsByDependency_DiamondDependency_IDMapping(t *testing.T) {
	t.Parallel()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Root", Description: "No deps"},
			{ID: "E-002", Title: "Left", Description: "Depends on E-001", DependenciesOnEpics: []string{"E-001"}},
			{ID: "E-003", Title: "Right", Description: "Depends on E-001", DependenciesOnEpics: []string{"E-001"}},
			{ID: "E-004", Title: "Sink", Description: "Depends on E-002 and E-003", DependenciesOnEpics: []string{"E-002", "E-003"}},
		},
	}

	order, err := SortEpicsByDependency(breakdown)
	require.NoError(t, err)

	results := map[string]*EpicTaskResult{
		"E-001": {EpicID: "E-001", Tasks: []TaskDef{
			{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
		"E-002": {EpicID: "E-002", Tasks: []TaskDef{
			{TempID: "E002-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
		"E-003": {EpicID: "E-003", Tasks: []TaskDef{
			{TempID: "E003-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
		"E-004": {EpicID: "E-004", Tasks: []TaskDef{
			{TempID: "E004-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	require.Len(t, merged, 4)
	// IDs are sequential starting at T-001.
	assert.Equal(t, "T-001", merged[0].GlobalID)
	assert.Equal(t, "T-002", merged[1].GlobalID)
	assert.Equal(t, "T-003", merged[2].GlobalID)
	assert.Equal(t, "T-004", merged[3].GlobalID)

	// E-001 is first in topo order.
	assert.Equal(t, "E-001", merged[0].EpicID)
	// E-004 is last in topo order.
	assert.Equal(t, "E-004", merged[3].EpicID)

	// Mapping completeness: all 4 temp_ids present.
	assert.Len(t, mapping, 4)
	assert.Equal(t, "T-001", mapping["E001-T01"])
	assert.Equal(t, "T-004", mapping["E004-T01"])
}

// TestAssignGlobalIDs_SingleEpic_FiveTasks verifies a single epic with exactly 5 tasks
// receives sequential IDs T-001 through T-005, as specified in T-060.
func TestAssignGlobalIDs_SingleEpic_FiveTasks(t *testing.T) {
	t.Parallel()

	order := []string{"E-001"}
	results := map[string]*EpicTaskResult{
		"E-001": {
			EpicID: "E-001",
			Tasks: []TaskDef{
				{TempID: "E001-T01", Title: "Task 1", Description: "First", AcceptanceCriteria: []string{"ac1"}, Effort: "small", Priority: "must-have"},
				{TempID: "E001-T02", Title: "Task 2", Description: "Second", AcceptanceCriteria: []string{"ac2"}, Effort: "small", Priority: "must-have"},
				{TempID: "E001-T03", Title: "Task 3", Description: "Third", AcceptanceCriteria: []string{"ac3"}, Effort: "medium", Priority: "should-have"},
				{TempID: "E001-T04", Title: "Task 4", Description: "Fourth", AcceptanceCriteria: []string{"ac4"}, Effort: "large", Priority: "nice-to-have"},
				{TempID: "E001-T05", Title: "Task 5", Description: "Fifth", AcceptanceCriteria: []string{"ac5"}, Effort: "small", Priority: "must-have"},
			},
		},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	require.Len(t, merged, 5)
	assert.Equal(t, "T-001", merged[0].GlobalID)
	assert.Equal(t, "T-002", merged[1].GlobalID)
	assert.Equal(t, "T-003", merged[2].GlobalID)
	assert.Equal(t, "T-004", merged[3].GlobalID)
	assert.Equal(t, "T-005", merged[4].GlobalID)

	// Original order within the epic is preserved.
	assert.Equal(t, "E001-T01", merged[0].TempID)
	assert.Equal(t, "E001-T02", merged[1].TempID)
	assert.Equal(t, "E001-T03", merged[2].TempID)
	assert.Equal(t, "E001-T04", merged[3].TempID)
	assert.Equal(t, "E001-T05", merged[4].TempID)

	// All 5 temp_ids must be in the mapping.
	assert.Len(t, mapping, 5)
	assert.Equal(t, "T-001", mapping["E001-T01"])
	assert.Equal(t, "T-002", mapping["E001-T02"])
	assert.Equal(t, "T-003", mapping["E001-T03"])
	assert.Equal(t, "T-004", mapping["E001-T04"])
	assert.Equal(t, "T-005", mapping["E001-T05"])
}

// TestSortEpicsByDependency_SelfDependencyCycle verifies that an epic listed as depending
// on itself is treated as a cycle and returns a clear error. The schema validator rejects
// self-references before reaching this function, but SortEpicsByDependency ignores
// self-references (epicSet[dep] is true for self) so the self-loop raises the in-degree
// and the epic is never enqueued — producing a cycle error.
func TestSortEpicsByDependency_SelfDependencyCycle(t *testing.T) {
	t.Parallel()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			// E-001 depends on itself — schema would reject this, but merger must handle it.
			{ID: "E-001", Title: "Self", Description: "Self-referential", DependenciesOnEpics: []string{"E-001"}},
			{ID: "E-002", Title: "Other", Description: "No deps"},
		},
	}

	order, err := SortEpicsByDependency(breakdown)
	require.Error(t, err)
	assert.Nil(t, order)
	assert.Contains(t, err.Error(), "cyclic epic dependency detected")
	assert.Contains(t, err.Error(), "E-001")
}

// TestSortEpicsByDependency_Deterministic verifies that running SortEpicsByDependency
// multiple times with identical input always produces the same result (no map iteration
// non-determinism leaks into the output).
func TestSortEpicsByDependency_Deterministic(t *testing.T) {
	t.Parallel()

	// Mix of root epics and epics with shared dependencies to exercise multiple
	// simultaneously-available-nodes code paths.
	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D"},
			{ID: "E-002", Title: "B", Description: "D"},
			{ID: "E-003", Title: "C", Description: "D", DependenciesOnEpics: []string{"E-001"}},
			{ID: "E-004", Title: "D", Description: "D", DependenciesOnEpics: []string{"E-001"}},
			{ID: "E-005", Title: "E", Description: "D", DependenciesOnEpics: []string{"E-002"}},
			{ID: "E-006", Title: "F", Description: "D", DependenciesOnEpics: []string{"E-003", "E-004"}},
		},
	}

	const iterations = 20
	first, err := SortEpicsByDependency(breakdown)
	require.NoError(t, err)

	for i := 1; i < iterations; i++ {
		got, err := SortEpicsByDependency(breakdown)
		require.NoError(t, err)
		assert.Equal(t, first, got, "iteration %d produced different order", i)
	}
}

// TestAssignGlobalIDs_IDMappingCompleteness verifies that every temp_id across all epics
// appears in the returned IDMapping with the correct sequential global_id.
func TestAssignGlobalIDs_IDMappingCompleteness(t *testing.T) {
	t.Parallel()

	order := []string{"E-001", "E-002", "E-003"}
	results := map[string]*EpicTaskResult{
		"E-001": {EpicID: "E-001", Tasks: []TaskDef{
			{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			{TempID: "E001-T02", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
		"E-002": {EpicID: "E-002", Tasks: []TaskDef{
			{TempID: "E002-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			{TempID: "E002-T02", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			{TempID: "E002-T03", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
		"E-003": {EpicID: "E-003", Tasks: []TaskDef{
			{TempID: "E003-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
	}

	// Collect all expected temp_ids.
	expectedTempIDs := []string{
		"E001-T01", "E001-T02",
		"E002-T01", "E002-T02", "E002-T03",
		"E003-T01",
	}

	merged, mapping := AssignGlobalIDs(order, results)

	// Mapping must contain every temp_id.
	require.Len(t, mapping, len(expectedTempIDs))
	for _, tempID := range expectedTempIDs {
		_, ok := mapping[tempID]
		assert.True(t, ok, "IDMapping missing temp_id %q", tempID)
	}

	// Mapping values must be the correct global IDs (sequential, no gaps).
	// Build expected mapping from merged slice.
	for _, mt := range merged {
		assert.Equal(t, mapping[mt.TempID], mt.GlobalID,
			"mapping[%q] should equal GlobalID %q", mt.TempID, mt.GlobalID)
	}

	// All global IDs must be unique and sequential.
	globalIDs := make([]string, len(merged))
	for i, mt := range merged {
		globalIDs[i] = mt.GlobalID
	}
	for i, id := range globalIDs {
		assert.Equal(t, fmt.Sprintf("T-%03d", i+1), id,
			"global ID at position %d should be T-%03d", i, i+1)
	}
}

// TestAssignGlobalIDs_TaskFieldsPreserved_AllFields verifies that ALL TaskDef fields
// (AcceptanceCriteria with multiple items, LocalDependencies, CrossEpicDeps) are correctly
// copied to MergedTask without truncation or mutation.
func TestAssignGlobalIDs_TaskFieldsPreserved_AllFields(t *testing.T) {
	t.Parallel()

	order := []string{"E-001"}
	results := map[string]*EpicTaskResult{
		"E-001": {
			EpicID: "E-001",
			Tasks: []TaskDef{
				{
					TempID:      "E001-T01",
					Title:       "Implement OAuth2 login flow",
					Description: "End-to-end OAuth2 with PKCE extension",
					AcceptanceCriteria: []string{
						"Authorization code flow works",
						"PKCE challenge validated server-side",
						"Tokens stored securely",
						"Refresh tokens rotated on use",
					},
					LocalDependencies: []string{"E001-T00"},
					CrossEpicDeps:     []string{"E-002:user-schema", "E-003:audit-log"},
					Effort:            "large",
					Priority:          "must-have",
				},
				{
					TempID:             "E001-T02",
					Title:              "Rate-limit login attempts",
					Description:        "Exponential back-off on failed attempts",
					AcceptanceCriteria: []string{"Five failures trigger lockout", "Lockout clears after 15 minutes"},
					LocalDependencies:  []string{},
					CrossEpicDeps:      []string{},
					Effort:             "medium",
					Priority:           "should-have",
				},
			},
		},
	}

	merged, _ := AssignGlobalIDs(order, results)

	require.Len(t, merged, 2)

	// First task: full field verification.
	mt1 := merged[0]
	assert.Equal(t, "T-001", mt1.GlobalID)
	assert.Equal(t, "E001-T01", mt1.TempID)
	assert.Equal(t, "E-001", mt1.EpicID)
	assert.Equal(t, "Implement OAuth2 login flow", mt1.Title)
	assert.Equal(t, "End-to-end OAuth2 with PKCE extension", mt1.Description)
	assert.Equal(t, []string{
		"Authorization code flow works",
		"PKCE challenge validated server-side",
		"Tokens stored securely",
		"Refresh tokens rotated on use",
	}, mt1.AcceptanceCriteria)
	assert.Equal(t, []string{"E001-T00"}, mt1.LocalDependencies)
	assert.Equal(t, []string{"E-002:user-schema", "E-003:audit-log"}, mt1.CrossEpicDeps)
	assert.Equal(t, "large", mt1.Effort)
	assert.Equal(t, "must-have", mt1.Priority)

	// Second task: verify empty slices are preserved faithfully.
	mt2 := merged[1]
	assert.Equal(t, "T-002", mt2.GlobalID)
	assert.Equal(t, "E001-T02", mt2.TempID)
	assert.Equal(t, "Rate-limit login attempts", mt2.Title)
	assert.Equal(t, []string{"Five failures trigger lockout", "Lockout clears after 15 minutes"}, mt2.AcceptanceCriteria)
	assert.Equal(t, []string{}, mt2.LocalDependencies)
	assert.Equal(t, []string{}, mt2.CrossEpicDeps)
	assert.Equal(t, "medium", mt2.Effort)
	assert.Equal(t, "should-have", mt2.Priority)
}

// TestAssignGlobalIDs_EmptyTempID_Skipped verifies the lenient path: tasks with an empty
// TempID are silently skipped and do not consume a global ID slot.
func TestAssignGlobalIDs_EmptyTempID_Skipped(t *testing.T) {
	t.Parallel()

	order := []string{"E-001"}
	results := map[string]*EpicTaskResult{
		"E-001": {
			EpicID: "E-001",
			Tasks: []TaskDef{
				{TempID: "E001-T01", Title: "Valid", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
				{TempID: "", Title: "Invalid (no temp_id)", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
				{TempID: "E001-T03", Title: "Also Valid", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			},
		},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	// The empty-TempID task is skipped; only 2 valid tasks are merged.
	require.Len(t, merged, 2)
	assert.Equal(t, "T-001", merged[0].GlobalID)
	assert.Equal(t, "E001-T01", merged[0].TempID)
	assert.Equal(t, "T-002", merged[1].GlobalID)
	assert.Equal(t, "E001-T03", merged[1].TempID)

	// Mapping contains only the two valid tasks.
	assert.Len(t, mapping, 2)
	assert.Equal(t, "T-001", mapping["E001-T01"])
	assert.Equal(t, "T-002", mapping["E001-T03"])
}

// TestAssignGlobalIDs_ThreeNoDeps_AllValidOrderings verifies that when three epics have
// no dependencies, the result is deterministic (lexicographic) and IDs are sequential.
func TestAssignGlobalIDs_ThreeNoDeps_AllValidOrderings(t *testing.T) {
	t.Parallel()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-003", Title: "C", Description: "D"},
			{ID: "E-001", Title: "A", Description: "D"},
			{ID: "E-002", Title: "B", Description: "D"},
		},
	}

	order, err := SortEpicsByDependency(breakdown)
	require.NoError(t, err)
	// Sorted lexicographically when all are roots.
	require.Equal(t, []string{"E-001", "E-002", "E-003"}, order)

	results := map[string]*EpicTaskResult{
		"E-001": {EpicID: "E-001", Tasks: []TaskDef{
			{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
		"E-002": {EpicID: "E-002", Tasks: []TaskDef{
			{TempID: "E002-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
		"E-003": {EpicID: "E-003", Tasks: []TaskDef{
			{TempID: "E003-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		}},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	require.Len(t, merged, 3)
	assert.Equal(t, "T-001", merged[0].GlobalID)
	assert.Equal(t, "E-001", merged[0].EpicID)
	assert.Equal(t, "T-002", merged[1].GlobalID)
	assert.Equal(t, "E-002", merged[1].EpicID)
	assert.Equal(t, "T-003", merged[2].GlobalID)
	assert.Equal(t, "E-003", merged[2].EpicID)

	assert.Len(t, mapping, 3)
	assert.Equal(t, "T-001", mapping["E001-T01"])
	assert.Equal(t, "T-002", mapping["E002-T01"])
	assert.Equal(t, "T-003", mapping["E003-T01"])
}

// TestSortEpicsByDependency_UnknownDependencyIgnored verifies that a dependency on an
// unknown epic ID (not in the breakdown) is silently ignored and does not inflate in-degree.
func TestSortEpicsByDependency_UnknownDependencyIgnored(t *testing.T) {
	t.Parallel()

	breakdown := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "A", Description: "D"},
			// E-002 lists E-999 which does not exist; it should be treated as a no-op dep.
			{ID: "E-002", Title: "B", Description: "D", DependenciesOnEpics: []string{"E-999"}},
		},
	}

	order, err := SortEpicsByDependency(breakdown)
	require.NoError(t, err)
	// Both epics have effective in-degree 0 (unknown dep ignored), sorted lexicographically.
	assert.Equal(t, []string{"E-001", "E-002"}, order)
}

// TestAssignGlobalIDs_LargeCount_IDFormat verifies the ID format boundary:
// 999 tasks use T-NNN (3 digits), 1000 tasks use T-NNNN (4 digits), and
// values above 1000 keep the 4-digit format.
func TestAssignGlobalIDs_LargeCount_IDFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		taskCount   int
		wantFirstID string
		wantLastID  string
	}{
		{name: "boundary_999_three_digits", taskCount: 999, wantFirstID: "T-001", wantLastID: "T-999"},
		{name: "boundary_1000_four_digits", taskCount: 1000, wantFirstID: "T-0001", wantLastID: "T-1000"},
		{name: "above_1000_four_digits", taskCount: 1001, wantFirstID: "T-0001", wantLastID: "T-1001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tasks := make([]TaskDef, tt.taskCount)
			for i := range tasks {
				tasks[i] = TaskDef{
					TempID:             fmt.Sprintf("E001-T%05d", i),
					Title:              "T",
					Description:        "D",
					AcceptanceCriteria: []string{"ac"},
					Effort:             "small",
					Priority:           "must-have",
				}
			}

			// Use unique TempIDs (the format above may collide at 100+ for the original
			// test helper pattern, so we use 5-digit suffix here).
			// Deduplicate in the unlikely case of collision.
			seen := make(map[string]bool, len(tasks))
			unique := tasks[:0]
			for _, task := range tasks {
				if !seen[task.TempID] {
					seen[task.TempID] = true
					unique = append(unique, task)
				}
			}
			tasks = unique

			order := []string{"E-001"}
			results := map[string]*EpicTaskResult{
				"E-001": {EpicID: "E-001", Tasks: tasks},
			}

			merged, _ := AssignGlobalIDs(order, results)

			require.Len(t, merged, tt.taskCount)
			assert.Equal(t, tt.wantFirstID, merged[0].GlobalID)
			assert.Equal(t, tt.wantLastID, merged[len(merged)-1].GlobalID)
		})
	}
}

// TestAssignGlobalIDs_GlobalIDsAreUnique verifies that every MergedTask receives a unique
// GlobalID even across multiple epics with many tasks.
func TestAssignGlobalIDs_GlobalIDsAreUnique(t *testing.T) {
	t.Parallel()

	order := []string{"E-001", "E-002", "E-003"}
	results := map[string]*EpicTaskResult{
		"E-001": {EpicID: "E-001", Tasks: buildTasks(t, "E001", 10)},
		"E-002": {EpicID: "E-002", Tasks: buildTasks(t, "E002", 15)},
		"E-003": {EpicID: "E-003", Tasks: buildTasks(t, "E003", 5)},
	}

	merged, mapping := AssignGlobalIDs(order, results)

	require.Len(t, merged, 30)
	assert.Len(t, mapping, 30)

	// Collect all global IDs and verify they are unique.
	globalIDSet := make(map[string]bool, len(merged))
	for _, mt := range merged {
		assert.False(t, globalIDSet[mt.GlobalID], "duplicate GlobalID: %s", mt.GlobalID)
		globalIDSet[mt.GlobalID] = true
	}
}

// TestAssignGlobalIDs_TaskOrderWithinEpicPreserved verifies that tasks within a single
// epic retain their original declaration order after global ID assignment.
func TestAssignGlobalIDs_TaskOrderWithinEpicPreserved(t *testing.T) {
	t.Parallel()

	// Define tasks in a deliberately non-alphabetical temp_id order to confirm
	// the function does not sort them internally.
	order := []string{"E-001"}
	results := map[string]*EpicTaskResult{
		"E-001": {
			EpicID: "E-001",
			Tasks: []TaskDef{
				{TempID: "E001-T03", Title: "Third declared", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
				{TempID: "E001-T01", Title: "First declared", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
				{TempID: "E001-T02", Title: "Second declared", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			},
		},
	}

	merged, _ := AssignGlobalIDs(order, results)

	require.Len(t, merged, 3)
	// The original declaration order must be preserved.
	assert.Equal(t, "E001-T03", merged[0].TempID)
	assert.Equal(t, "T-001", merged[0].GlobalID)

	assert.Equal(t, "E001-T01", merged[1].TempID)
	assert.Equal(t, "T-002", merged[1].GlobalID)

	assert.Equal(t, "E001-T02", merged[2].TempID)
	assert.Equal(t, "T-003", merged[2].GlobalID)
}

// TestSortEpicsByDependency_NoDeps_LexicographicOrder verifies that epics with no
// dependencies are always sorted lexicographically regardless of declaration order.
func TestSortEpicsByDependency_NoDeps_LexicographicOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		epicIDs   []string
		wantOrder []string
	}{
		{
			name:      "already sorted",
			epicIDs:   []string{"E-001", "E-002", "E-003"},
			wantOrder: []string{"E-001", "E-002", "E-003"},
		},
		{
			name:      "reverse sorted",
			epicIDs:   []string{"E-003", "E-002", "E-001"},
			wantOrder: []string{"E-001", "E-002", "E-003"},
		},
		{
			name:      "single epic",
			epicIDs:   []string{"E-042"},
			wantOrder: []string{"E-042"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			epics := make([]Epic, len(tt.epicIDs))
			for i, id := range tt.epicIDs {
				epics[i] = Epic{ID: id, Title: "T", Description: "D"}
			}

			order, err := SortEpicsByDependency(&EpicBreakdown{Epics: epics})
			require.NoError(t, err)
			assert.Equal(t, tt.wantOrder, order)
		})
	}
}

// TestSortEpicsByDependency_CycleError_ContainsAllCyclicEpics verifies that when multiple
// epics form a cycle, the error message contains ALL epic IDs involved in the cycle.
func TestSortEpicsByDependency_CycleError_ContainsAllCyclicEpics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		breakdown     *EpicBreakdown
		wantInMessage []string
	}{
		{
			name: "two-node cycle",
			breakdown: &EpicBreakdown{
				Epics: []Epic{
					{ID: "E-001", Title: "A", Description: "D", DependenciesOnEpics: []string{"E-002"}},
					{ID: "E-002", Title: "B", Description: "D", DependenciesOnEpics: []string{"E-001"}},
				},
			},
			wantInMessage: []string{"E-001", "E-002", "cyclic epic dependency detected"},
		},
		{
			name: "three-node cycle",
			breakdown: &EpicBreakdown{
				Epics: []Epic{
					{ID: "E-001", Title: "A", Description: "D", DependenciesOnEpics: []string{"E-003"}},
					{ID: "E-002", Title: "B", Description: "D", DependenciesOnEpics: []string{"E-001"}},
					{ID: "E-003", Title: "C", Description: "D", DependenciesOnEpics: []string{"E-002"}},
				},
			},
			wantInMessage: []string{"E-001", "E-002", "E-003", "form a cycle"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := SortEpicsByDependency(tt.breakdown)
			require.Error(t, err)
			for _, want := range tt.wantInMessage {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

// TestAssignGlobalIDs_IDMapping_ValuesSorted verifies that the values in IDMapping
// are sorted consistently (T-001 < T-002 < ... T-NNN) when sorted by their keys.
func TestAssignGlobalIDs_IDMapping_ValuesSorted(t *testing.T) {
	t.Parallel()

	order := []string{"E-001"}
	results := map[string]*EpicTaskResult{
		"E-001": {EpicID: "E-001", Tasks: buildTasks(t, "E001", 10)},
	}

	merged, mapping := AssignGlobalIDs(order, results)
	require.Len(t, merged, 10)

	// Collect all mapping values and verify they are the same set as the global IDs.
	values := make([]string, 0, len(mapping))
	for _, v := range mapping {
		values = append(values, v)
	}
	sort.Strings(values)

	expected := make([]string, len(merged))
	for i, mt := range merged {
		expected[i] = mt.GlobalID
	}
	sort.Strings(expected)

	assert.Equal(t, expected, values)
}

// --- RemapDependencies tests ---

// TestRemapDependencies_EmptyTasks verifies that an empty task slice produces an empty
// result and a zeroed report.
func TestRemapDependencies_EmptyTasks(t *testing.T) {
	t.Parallel()

	updated, report := RemapDependencies(nil, IDMapping{}, nil)
	assert.Empty(t, updated)
	assert.Equal(t, 0, report.Remapped)
	assert.Empty(t, report.Unresolved)
	assert.Empty(t, report.Ambiguous)
}

// TestRemapDependencies_NoDeps verifies tasks with no LocalDependencies or CrossEpicDeps
// receive an empty Dependencies slice and the report shows zero remapped.
func TestRemapDependencies_NoDeps(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", TempID: "E001-T01", EpicID: "E-001", Title: "Alpha"},
		{GlobalID: "T-002", TempID: "E001-T02", EpicID: "E-001", Title: "Beta"},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E001-T02": "T-002",
	}
	epicTasks := map[string][]MergedTask{"E-001": tasks}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 2)
	assert.Empty(t, updated[0].Dependencies)
	assert.Empty(t, updated[1].Dependencies)
	assert.Equal(t, 0, report.Remapped)
	assert.Empty(t, report.Unresolved)
	assert.Empty(t, report.Ambiguous)
}

// TestRemapDependencies_LocalDeps_Resolved verifies that local temp_id dependencies
// are correctly translated to global IDs.
func TestRemapDependencies_LocalDeps_Resolved(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", TempID: "E001-T01", EpicID: "E-001", Title: "First"},
		{
			GlobalID:          "T-002",
			TempID:            "E001-T02",
			EpicID:            "E-001",
			Title:             "Second",
			LocalDependencies: []string{"E001-T01"},
		},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E001-T02": "T-002",
	}
	epicTasks := map[string][]MergedTask{"E-001": tasks}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 2)
	assert.Empty(t, updated[0].Dependencies)
	assert.Equal(t, []string{"T-001"}, updated[1].Dependencies)
	assert.Equal(t, 1, report.Remapped)
	assert.Empty(t, report.Unresolved)
}

// TestRemapDependencies_LocalDep_Unresolved verifies that a local dep referencing an
// unknown temp_id is added to report.Unresolved and not included in Dependencies.
func TestRemapDependencies_LocalDep_Unresolved(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{
			GlobalID:          "T-001",
			TempID:            "E001-T01",
			EpicID:            "E-001",
			Title:             "Alpha",
			LocalDependencies: []string{"E001-T99"}, // unknown
		},
	}
	mapping := IDMapping{"E001-T01": "T-001"}
	epicTasks := map[string][]MergedTask{"E-001": tasks}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 1)
	assert.Empty(t, updated[0].Dependencies)
	assert.Equal(t, 0, report.Remapped)
	require.Len(t, report.Unresolved, 1)
	assert.Equal(t, "T-001", report.Unresolved[0].TaskID)
	assert.Equal(t, "E001-T99", report.Unresolved[0].Reference)
}

// TestRemapDependencies_LocalDep_SelfReference verifies that a local dep whose resolved
// global ID equals the task's own global ID is silently dropped (no self-dep, no unresolved).
func TestRemapDependencies_LocalDep_SelfReference(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{
			GlobalID:          "T-001",
			TempID:            "E001-T01",
			EpicID:            "E-001",
			Title:             "Self",
			LocalDependencies: []string{"E001-T01"}, // maps to its own global ID
		},
	}
	mapping := IDMapping{"E001-T01": "T-001"}
	epicTasks := map[string][]MergedTask{"E-001": tasks}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 1)
	assert.Empty(t, updated[0].Dependencies, "self-reference must be dropped")
	assert.Equal(t, 0, report.Remapped)
	assert.Empty(t, report.Unresolved)
}

// TestRemapDependencies_CrossEpicDep_Resolved verifies that a cross-epic dep resolves
// correctly via exact title match.
func TestRemapDependencies_CrossEpicDep_Resolved(t *testing.T) {
	t.Parallel()

	e002Tasks := []MergedTask{
		{GlobalID: "T-010", TempID: "E002-T01", EpicID: "E-002", Title: "Database schema"},
	}
	tasks := []MergedTask{
		{
			GlobalID:      "T-001",
			TempID:        "E001-T01",
			EpicID:        "E-001",
			Title:         "App bootstrap",
			CrossEpicDeps: []string{"E-002:database schema"},
		},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E002-T01": "T-010",
	}
	epicTasks := map[string][]MergedTask{
		"E-001": tasks,
		"E-002": e002Tasks,
	}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 1)
	assert.Equal(t, []string{"T-010"}, updated[0].Dependencies)
	assert.Equal(t, 1, report.Remapped)
	assert.Empty(t, report.Unresolved)
	assert.Empty(t, report.Ambiguous)
}

// TestRemapDependencies_CrossEpicDep_SlugLabel verifies that a dash-separated slug label
// resolves against a space-separated title (substring match).
func TestRemapDependencies_CrossEpicDep_SlugLabel(t *testing.T) {
	t.Parallel()

	e002Tasks := []MergedTask{
		{GlobalID: "T-010", TempID: "E002-T01", EpicID: "E-002", Title: "Set up database schema"},
	}
	tasks := []MergedTask{
		{
			GlobalID:      "T-001",
			TempID:        "E001-T01",
			EpicID:        "E-001",
			Title:         "Auth",
			CrossEpicDeps: []string{"E-002:database schema"},
		},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E002-T01": "T-010",
	}
	epicTasks := map[string][]MergedTask{
		"E-001": tasks,
		"E-002": e002Tasks,
	}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 1)
	assert.Equal(t, []string{"T-010"}, updated[0].Dependencies)
	assert.Equal(t, 1, report.Remapped)
	assert.Empty(t, report.Unresolved)
}

// TestRemapDependencies_CrossEpicDep_UnknownEpic verifies that a cross-epic dep referencing
// an epic that is not in epicTasks is added to report.Unresolved.
func TestRemapDependencies_CrossEpicDep_UnknownEpic(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{
			GlobalID:      "T-001",
			TempID:        "E001-T01",
			EpicID:        "E-001",
			Title:         "Task",
			CrossEpicDeps: []string{"E-999:nonexistent"},
		},
	}
	mapping := IDMapping{"E001-T01": "T-001"}
	epicTasks := map[string][]MergedTask{"E-001": tasks}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 1)
	assert.Empty(t, updated[0].Dependencies)
	assert.Equal(t, 0, report.Remapped)
	require.Len(t, report.Unresolved, 1)
	assert.Equal(t, "T-001", report.Unresolved[0].TaskID)
	assert.Equal(t, "E-999:nonexistent", report.Unresolved[0].Reference)
}

// TestRemapDependencies_CrossEpicDep_UnknownLabel verifies that a cross-epic dep where
// the label matches no task title in the target epic is added to report.Unresolved.
func TestRemapDependencies_CrossEpicDep_UnknownLabel(t *testing.T) {
	t.Parallel()

	e002Tasks := []MergedTask{
		{GlobalID: "T-010", TempID: "E002-T01", EpicID: "E-002", Title: "Database schema"},
	}
	tasks := []MergedTask{
		{
			GlobalID:      "T-001",
			TempID:        "E001-T01",
			EpicID:        "E-001",
			Title:         "Auth",
			CrossEpicDeps: []string{"E-002:totally-unknown-label"},
		},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E002-T01": "T-010",
	}
	epicTasks := map[string][]MergedTask{
		"E-001": tasks,
		"E-002": e002Tasks,
	}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 1)
	assert.Empty(t, updated[0].Dependencies)
	assert.Equal(t, 0, report.Remapped)
	require.Len(t, report.Unresolved, 1)
	assert.Equal(t, "E-002:totally-unknown-label", report.Unresolved[0].Reference)
}

// TestRemapDependencies_CrossEpicDep_Ambiguous verifies that when a label matches multiple
// task titles in the target epic, the ambiguity is recorded and the first candidate is used.
func TestRemapDependencies_CrossEpicDep_Ambiguous(t *testing.T) {
	t.Parallel()

	// Two tasks in E-002 whose titles both contain "schema".
	e002Tasks := []MergedTask{
		{GlobalID: "T-010", TempID: "E002-T01", EpicID: "E-002", Title: "Database schema setup"},
		{GlobalID: "T-011", TempID: "E002-T02", EpicID: "E-002", Title: "API schema definition"},
	}
	tasks := []MergedTask{
		{
			GlobalID:      "T-001",
			TempID:        "E001-T01",
			EpicID:        "E-001",
			Title:         "Auth",
			CrossEpicDeps: []string{"E-002:schema"},
		},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E002-T01": "T-010",
		"E002-T02": "T-011",
	}
	epicTasks := map[string][]MergedTask{
		"E-001": tasks,
		"E-002": e002Tasks,
	}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 1)
	// A best-guess candidate should still be recorded in Dependencies.
	assert.Len(t, updated[0].Dependencies, 1)
	assert.Equal(t, 1, report.Remapped)
	assert.Empty(t, report.Unresolved)
	require.Len(t, report.Ambiguous, 1)
	assert.Equal(t, "T-001", report.Ambiguous[0].TaskID)
	assert.Equal(t, "E-002:schema", report.Ambiguous[0].Reference)
	assert.Len(t, report.Ambiguous[0].Candidates, 2)
}

// TestRemapDependencies_Deduplication verifies that the same global ID appearing in both
// LocalDependencies and CrossEpicDeps is only listed once in Dependencies.
func TestRemapDependencies_Deduplication(t *testing.T) {
	t.Parallel()

	// T-010 appears as both a local dep (via temp_id) and a cross-epic dep (via title).
	e002Tasks := []MergedTask{
		{GlobalID: "T-010", TempID: "E002-T01", EpicID: "E-002", Title: "Shared service"},
	}
	tasks := []MergedTask{
		{
			GlobalID:          "T-001",
			TempID:            "E001-T01",
			EpicID:            "E-001",
			Title:             "Consumer",
			LocalDependencies: []string{"E002-T01"},             // resolves to T-010
			CrossEpicDeps:     []string{"E-002:shared service"}, // also resolves to T-010
		},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E002-T01": "T-010",
	}
	epicTasks := map[string][]MergedTask{
		"E-001": tasks,
		"E-002": e002Tasks,
	}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 1)
	// T-010 should appear exactly once.
	assert.Equal(t, []string{"T-010"}, updated[0].Dependencies)
	// Remapped should count the first resolution; the duplicate is silently dropped.
	assert.Equal(t, 1, report.Remapped)
}

// TestRemapDependencies_MultipleLocalDeps_AllResolved verifies that a task with multiple
// local deps gets all of them resolved in order.
func TestRemapDependencies_MultipleLocalDeps_AllResolved(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", TempID: "E001-T01", EpicID: "E-001", Title: "First"},
		{GlobalID: "T-002", TempID: "E001-T02", EpicID: "E-001", Title: "Second"},
		{
			GlobalID:          "T-003",
			TempID:            "E001-T03",
			EpicID:            "E-001",
			Title:             "Third",
			LocalDependencies: []string{"E001-T01", "E001-T02"},
		},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E001-T02": "T-002",
		"E001-T03": "T-003",
	}
	epicTasks := map[string][]MergedTask{"E-001": tasks}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 3)
	assert.Equal(t, []string{"T-001", "T-002"}, updated[2].Dependencies)
	assert.Equal(t, 2, report.Remapped)
	assert.Empty(t, report.Unresolved)
}

// TestRemapDependencies_CrossEpicDep_ColonInLabel verifies that a label containing a
// colon (after the first split) is handled correctly; only the first colon is used as
// the separator between epic ID and label.
func TestRemapDependencies_CrossEpicDep_ColonInLabel(t *testing.T) {
	t.Parallel()

	e002Tasks := []MergedTask{
		{GlobalID: "T-010", TempID: "E002-T01", EpicID: "E-002", Title: "http: server setup"},
	}
	tasks := []MergedTask{
		{
			GlobalID:      "T-001",
			TempID:        "E001-T01",
			EpicID:        "E-001",
			Title:         "Client",
			CrossEpicDeps: []string{"E-002:http: server setup"},
		},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E002-T01": "T-010",
	}
	epicTasks := map[string][]MergedTask{
		"E-001": tasks,
		"E-002": e002Tasks,
	}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 1)
	assert.Equal(t, []string{"T-010"}, updated[0].Dependencies)
	assert.Equal(t, 1, report.Remapped)
	assert.Empty(t, report.Unresolved)
}

// TestRemapDependencies_MixedLocalAndCrossEpic verifies that a task with both local
// and cross-epic deps accumulates all resolved IDs into a single Dependencies slice.
func TestRemapDependencies_MixedLocalAndCrossEpic(t *testing.T) {
	t.Parallel()

	e002Tasks := []MergedTask{
		{GlobalID: "T-010", TempID: "E002-T01", EpicID: "E-002", Title: "Auth service"},
	}
	tasks := []MergedTask{
		{GlobalID: "T-001", TempID: "E001-T01", EpicID: "E-001", Title: "Setup"},
		{
			GlobalID:          "T-002",
			TempID:            "E001-T02",
			EpicID:            "E-001",
			Title:             "Main",
			LocalDependencies: []string{"E001-T01"},
			CrossEpicDeps:     []string{"E-002:auth service"},
		},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E001-T02": "T-002",
		"E002-T01": "T-010",
	}
	epicTasks := map[string][]MergedTask{
		"E-001": tasks,
		"E-002": e002Tasks,
	}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 2)
	assert.Empty(t, updated[0].Dependencies)
	assert.Equal(t, []string{"T-001", "T-010"}, updated[1].Dependencies)
	assert.Equal(t, 2, report.Remapped)
	assert.Empty(t, report.Unresolved)
	assert.Empty(t, report.Ambiguous)
}

// TestRemapDependencies_OriginalTasksUnmutated verifies that the input tasks slice is not
// mutated by RemapDependencies (the returned slice is a copy).
func TestRemapDependencies_OriginalTasksUnmutated(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{
			GlobalID:          "T-001",
			TempID:            "E001-T01",
			EpicID:            "E-001",
			Title:             "Alpha",
			LocalDependencies: []string{"E001-T02"},
		},
		{GlobalID: "T-002", TempID: "E001-T02", EpicID: "E-001", Title: "Beta"},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E001-T02": "T-002",
	}
	epicTasks := map[string][]MergedTask{"E-001": tasks}

	// Record the original Dependencies field before the call.
	origDeps := tasks[0].Dependencies

	_, _ = RemapDependencies(tasks, mapping, epicTasks)

	// The original slice must not have been modified.
	assert.Equal(t, origDeps, tasks[0].Dependencies,
		"input tasks[0].Dependencies must not be mutated")
}

// TestRemapDependencies_CrossEpicDep_SelfReference verifies that a cross-epic dep that
// resolves to the task's own global ID is silently dropped (no self-dep in Dependencies).
func TestRemapDependencies_CrossEpicDep_SelfReference(t *testing.T) {
	t.Parallel()

	// T-001 is in E-001 but also appears in the E-002 epicTasks map (edge-case: caller
	// included the wrong task); the key point is that if the resolved global ID matches
	// the processing task's own GlobalID, it should be skipped.
	e001Tasks := []MergedTask{
		{GlobalID: "T-001", TempID: "E001-T01", EpicID: "E-001", Title: "Self task"},
	}
	tasks := []MergedTask{
		{
			GlobalID:      "T-001",
			TempID:        "E001-T01",
			EpicID:        "E-001",
			Title:         "Self task",
			CrossEpicDeps: []string{"E-001:self task"},
		},
	}
	mapping := IDMapping{"E001-T01": "T-001"}
	epicTasks := map[string][]MergedTask{"E-001": e001Tasks}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 1)
	assert.Empty(t, updated[0].Dependencies, "self cross-epic ref must be dropped")
	assert.Equal(t, 0, report.Remapped)
	assert.Empty(t, report.Unresolved)
}

// TestRemapDependencies_TitleNormalization verifies that title matching is case-insensitive
// and trims surrounding whitespace.
func TestRemapDependencies_TitleNormalization(t *testing.T) {
	t.Parallel()

	e002Tasks := []MergedTask{
		{GlobalID: "T-010", TempID: "E002-T01", EpicID: "E-002", Title: "  Set Up Database Schema  "},
	}
	tasks := []MergedTask{
		{
			GlobalID:      "T-001",
			TempID:        "E001-T01",
			EpicID:        "E-001",
			Title:         "Migrator",
			CrossEpicDeps: []string{"E-002:database schema"},
		},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E002-T01": "T-010",
	}
	epicTasks := map[string][]MergedTask{
		"E-001": tasks,
		"E-002": e002Tasks,
	}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 1)
	assert.Equal(t, []string{"T-010"}, updated[0].Dependencies)
	assert.Equal(t, 1, report.Remapped)
}

// TestRemapDependencies_MultipleUnresolved verifies that multiple unresolved refs from
// multiple tasks are all captured in the report.
func TestRemapDependencies_MultipleUnresolved(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{
			GlobalID:          "T-001",
			TempID:            "E001-T01",
			EpicID:            "E-001",
			Title:             "Alpha",
			LocalDependencies: []string{"E001-T99"},
			CrossEpicDeps:     []string{"E-999:ghost"},
		},
		{
			GlobalID:          "T-002",
			TempID:            "E001-T02",
			EpicID:            "E-001",
			Title:             "Beta",
			LocalDependencies: []string{"E001-T88"},
		},
	}
	mapping := IDMapping{
		"E001-T01": "T-001",
		"E001-T02": "T-002",
	}
	epicTasks := map[string][]MergedTask{"E-001": tasks}

	updated, report := RemapDependencies(tasks, mapping, epicTasks)

	require.Len(t, updated, 2)
	assert.Empty(t, updated[0].Dependencies)
	assert.Empty(t, updated[1].Dependencies)
	assert.Equal(t, 0, report.Remapped)
	assert.Len(t, report.Unresolved, 3)
}

// --- NormalizeTitle tests ---

func TestNormalizeTitle_Lowercase(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "user authentication", NormalizeTitle("User Authentication"))
}

func TestNormalizeTitle_StripImplementPrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "user auth", NormalizeTitle("Implement user auth"))
}

func TestNormalizeTitle_StripCreatePrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "database schema", NormalizeTitle("Create database schema"))
}

func TestNormalizeTitle_StripSetUpPrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "database", NormalizeTitle("Set up database"))
}

func TestNormalizeTitle_StripAddPrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "logging", NormalizeTitle("Add logging"))
}

func TestNormalizeTitle_StripBuildPrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "cli interface", NormalizeTitle("Build CLI interface"))
}

func TestNormalizeTitle_StripDefinePrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "api contract", NormalizeTitle("Define API contract"))
}

func TestNormalizeTitle_StripWritePrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "unit tests", NormalizeTitle("Write unit tests"))
}

func TestNormalizeTitle_StripConfigurePrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "ci pipeline", NormalizeTitle("Configure CI pipeline"))
}

func TestNormalizeTitle_StripDesignPrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "data model", NormalizeTitle("Design data model"))
}

func TestNormalizeTitle_StripEstablishPrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "coding standards", NormalizeTitle("Establish coding standards"))
}

// TestNormalizeTitle_ImplementationNotStripped verifies that "implementation" is NOT
// stripped because the next character after "implement" is a letter, not a space.
func TestNormalizeTitle_ImplementationNotStripped(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "implementation details", NormalizeTitle("Implementation details"))
}

// TestNormalizeTitle_PrefixOnlyFallback verifies that a title that is entirely the
// prefix word (e.g., "Implement") is returned unchanged after normalization, not as empty.
func TestNormalizeTitle_PrefixOnlyFallback(t *testing.T) {
	t.Parallel()

	// "implement" lowercased -> strips "implement" -> rest is "" -> fallback to original lowercased "implement"
	result := NormalizeTitle("Implement")
	assert.NotEmpty(t, result, "result must not be empty for prefix-only title")
	assert.Equal(t, "implement", result)
}

func TestNormalizeTitle_CollapseWhitespace(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "user auth service", NormalizeTitle("User   Auth   Service"))
}

func TestNormalizeTitle_RemovePunctuation(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "user auth", NormalizeTitle("User Auth!"))
}

func TestNormalizeTitle_RemovePunctuationDashes(t *testing.T) {
	t.Parallel()

	// Hyphens are punctuation and are removed; "Set-up" -> "setup" (not "set up").
	assert.Equal(t, "setup database", NormalizeTitle("Set-up database"))
}

func TestNormalizeTitle_EmptyString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", NormalizeTitle(""))
}

// TestNormalizeTitle_Table tests a broader set of NormalizeTitle inputs
// to guard against regressions.
func TestNormalizeTitle_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "implement prefix stripped",
			input: "Implement user authentication",
			want:  "user authentication",
		},
		{
			name:  "implementation not stripped",
			input: "Implementation of auth",
			want:  "implementation of auth",
		},
		{
			name:  "set up stripped (two words)",
			input: "Set up Redis cache",
			want:  "redis cache",
		},
		{
			name:  "create stripped",
			input: "Create REST endpoints",
			want:  "rest endpoints",
		},
		{
			name:  "add stripped",
			input: "Add error handling",
			want:  "error handling",
		},
		{
			name:  "build stripped",
			input: "Build the CLI parser",
			want:  "the cli parser",
		},
		{
			name:  "define stripped",
			input: "Define task interfaces",
			want:  "task interfaces",
		},
		{
			name:  "write stripped",
			input: "Write integration tests",
			want:  "integration tests",
		},
		{
			name:  "configure stripped",
			input: "Configure the database",
			want:  "the database",
		},
		{
			name:  "design stripped",
			input: "Design the schema",
			want:  "the schema",
		},
		{
			name:  "establish stripped",
			input: "Establish error conventions",
			want:  "error conventions",
		},
		{
			name:  "no prefix stripped",
			input: "User login flow",
			want:  "user login flow",
		},
		{
			name:  "punctuation removed",
			input: "Set up CI/CD pipeline",
			want:  "cicd pipeline",
		},
		{
			name:  "prefix only fallback",
			input: "Implement",
			want:  "implement",
		},
		{
			name:  "already lowercase no prefix",
			input: "auth middleware",
			want:  "auth middleware",
		},
		{
			name:  "mixed case with punctuation",
			input: "Create OAuth2.0 flow",
			want:  "oauth20 flow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NormalizeTitle(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- DeduplicateTasks tests ---

func TestDeduplicateTasks_EmptyInput(t *testing.T) {
	t.Parallel()

	out, report := DeduplicateTasks(nil)
	assert.Nil(t, out)
	assert.Equal(t, 0, report.OriginalCount)
	assert.Equal(t, 0, report.RemovedCount)
	assert.Equal(t, 0, report.FinalCount)
	assert.Empty(t, report.Merges)
}

func TestDeduplicateTasks_NoDuplicates(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "User authentication", AcceptanceCriteria: []string{"ac1"}},
		{GlobalID: "T-002", Title: "Database schema", AcceptanceCriteria: []string{"ac2"}},
		{GlobalID: "T-003", Title: "CI pipeline", AcceptanceCriteria: []string{"ac3"}},
	}

	out, report := DeduplicateTasks(tasks)

	require.Len(t, out, 3)
	assert.Equal(t, 3, report.OriginalCount)
	assert.Equal(t, 0, report.RemovedCount)
	assert.Equal(t, 3, report.FinalCount)
	assert.Empty(t, report.Merges)
}

func TestDeduplicateTasks_SingleTask(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Implement auth", AcceptanceCriteria: []string{"ac1"}},
	}

	out, report := DeduplicateTasks(tasks)

	require.Len(t, out, 1)
	assert.Equal(t, "T-001", out[0].GlobalID)
	assert.Equal(t, 1, report.OriginalCount)
	assert.Equal(t, 0, report.RemovedCount)
	assert.Equal(t, 1, report.FinalCount)
}

// TestDeduplicateTasks_ExactDuplicateTitles verifies that two tasks with identical
// titles are deduplicated, keeping the one with the lowest GlobalID.
func TestDeduplicateTasks_ExactDuplicateTitles(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Implement user auth", AcceptanceCriteria: []string{"ac1"}},
		{GlobalID: "T-002", Title: "Implement user auth", AcceptanceCriteria: []string{"ac2"}},
	}

	out, report := DeduplicateTasks(tasks)

	require.Len(t, out, 1)
	assert.Equal(t, "T-001", out[0].GlobalID, "keeper must be the task with lowest GlobalID")
	assert.Equal(t, 2, report.OriginalCount)
	assert.Equal(t, 1, report.RemovedCount)
	assert.Equal(t, 1, report.FinalCount)
	require.Len(t, report.Merges, 1)
	assert.Equal(t, "T-001", report.Merges[0].KeptTaskID)
	assert.Equal(t, []string{"T-002"}, report.Merges[0].RemovedTaskIDs)
}

// TestDeduplicateTasks_NormalizedDuplicates verifies that tasks with titles that normalize
// to the same string are treated as duplicates ("Implement user auth" == "Create user auth"
// ONLY if they normalize identically -- they won't here, so let's use same-prefix variant).
func TestDeduplicateTasks_NormalizedDuplicates_PrefixVariants(t *testing.T) {
	t.Parallel()

	// Both normalize to "user auth": "Implement user auth" -> "user auth", "Create user auth" -> "user auth"
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Implement user auth", AcceptanceCriteria: []string{"tokens validated"}},
		{GlobalID: "T-005", Title: "Create user auth", AcceptanceCriteria: []string{"tokens expired rejected"}},
	}

	out, report := DeduplicateTasks(tasks)

	require.Len(t, out, 1)
	assert.Equal(t, "T-001", out[0].GlobalID)
	assert.Equal(t, 1, report.RemovedCount)
	// The unique AC from T-005 must be merged into T-001.
	assert.Contains(t, out[0].AcceptanceCriteria, "tokens validated")
	assert.Contains(t, out[0].AcceptanceCriteria, "tokens expired rejected")
}

// TestDeduplicateTasks_AcceptanceCriteriaMerged verifies that unique ACs from removed
// tasks are appended to the keeper and duplicate ACs are not added twice.
func TestDeduplicateTasks_AcceptanceCriteriaMerged(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{
			GlobalID:           "T-001",
			Title:              "Implement auth middleware",
			AcceptanceCriteria: []string{"tokens validated", "expired tokens rejected"},
		},
		{
			GlobalID:           "T-003",
			Title:              "Add auth middleware",
			AcceptanceCriteria: []string{"tokens validated", "rate limiting applied"},
		},
	}

	out, report := DeduplicateTasks(tasks)

	require.Len(t, out, 1)
	keeper := out[0]
	assert.Equal(t, "T-001", keeper.GlobalID)

	// "tokens validated" is shared — must appear exactly once.
	count := 0
	for _, ac := range keeper.AcceptanceCriteria {
		if ac == "tokens validated" {
			count++
		}
	}
	assert.Equal(t, 1, count, "shared AC must not be duplicated")

	// "rate limiting applied" is unique to the removed task — must be merged.
	assert.Contains(t, keeper.AcceptanceCriteria, "rate limiting applied")

	assert.Equal(t, 1, report.Merges[0].MergedCriteria)
}

// TestDeduplicateTasks_DependencyRewrite verifies that dependency references pointing
// to a removed task are rewritten to the keeper's GlobalID.
func TestDeduplicateTasks_DependencyRewrite(t *testing.T) {
	t.Parallel()

	// T-001 and T-002 are duplicates (same normalized title). T-003 depends on T-002.
	// After dedup: T-002 is removed, T-003's dep must be rewritten to T-001.
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Implement auth", AcceptanceCriteria: []string{"ac1"}},
		{GlobalID: "T-002", Title: "Create auth", AcceptanceCriteria: []string{"ac2"}},
		{GlobalID: "T-003", Title: "Auth tests", AcceptanceCriteria: []string{"ac3"}, Dependencies: []string{"T-002"}},
	}

	out, report := DeduplicateTasks(tasks)

	require.Len(t, out, 2)

	// T-003 must now depend on T-001 (the keeper).
	var t003 *MergedTask
	for i := range out {
		if out[i].GlobalID == "T-003" {
			t003 = &out[i]
			break
		}
	}
	require.NotNil(t, t003)
	assert.Equal(t, []string{"T-001"}, t003.Dependencies)
	assert.Equal(t, 1, report.RewrittenDeps)
}

// TestDeduplicateTasks_DependencyRewrite_SelfRef verifies that a dependency that is
// rewritten to point to the keeper's own GlobalID is dropped (no self-reference).
func TestDeduplicateTasks_DependencyRewrite_SelfRef(t *testing.T) {
	t.Parallel()

	// T-001 is the keeper; T-002 is removed. T-001 has a dep on T-002, which would
	// become a self-ref after rewriting to T-001 — it must be dropped.
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Implement auth", AcceptanceCriteria: []string{"ac1"}, Dependencies: []string{"T-002"}},
		{GlobalID: "T-002", Title: "Create auth", AcceptanceCriteria: []string{"ac2"}},
	}

	out, report := DeduplicateTasks(tasks)

	require.Len(t, out, 1)
	assert.Empty(t, out[0].Dependencies, "self-reference must be dropped after rewrite")
	assert.Equal(t, 1, report.RewrittenDeps)
}

// TestDeduplicateTasks_DepRewrite_DeduplicatesDeps verifies that if two removed tasks
// both pointed to the same keeper, the dep list stays deduplicated.
func TestDeduplicateTasks_DepRewrite_DeduplicatesDeps(t *testing.T) {
	t.Parallel()

	// T-001 and T-002 both normalize to the same title; T-004 depends on both T-001 and T-002.
	// After dedup T-002 is removed and both deps rewrite to T-001 -> deduplicated to one entry.
	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Implement auth", AcceptanceCriteria: []string{"ac1"}},
		{GlobalID: "T-002", Title: "Create auth", AcceptanceCriteria: []string{"ac2"}},
		{GlobalID: "T-004", Title: "Auth tests", AcceptanceCriteria: []string{"ac4"}, Dependencies: []string{"T-001", "T-002"}},
	}

	out, report := DeduplicateTasks(tasks)

	require.Len(t, out, 2)

	var t004 *MergedTask
	for i := range out {
		if out[i].GlobalID == "T-004" {
			t004 = &out[i]
			break
		}
	}
	require.NotNil(t, t004)
	// T-001 already present, T-002 rewrites to T-001 -> only one T-001 entry.
	assert.Equal(t, []string{"T-001"}, t004.Dependencies)
	assert.Equal(t, 1, report.RewrittenDeps)
}

// TestDeduplicateTasks_OrderPreserved verifies that the output preserves the original
// input order of keeper tasks.
func TestDeduplicateTasks_OrderPreserved(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Alpha", AcceptanceCriteria: []string{"ac1"}},
		{GlobalID: "T-002", Title: "Beta", AcceptanceCriteria: []string{"ac2"}},
		{GlobalID: "T-003", Title: "Implement Alpha", AcceptanceCriteria: []string{"ac3"}}, // dup of T-001
		{GlobalID: "T-004", Title: "Gamma", AcceptanceCriteria: []string{"ac4"}},
	}

	out, report := DeduplicateTasks(tasks)

	require.Len(t, out, 3)
	assert.Equal(t, "T-001", out[0].GlobalID)
	assert.Equal(t, "T-002", out[1].GlobalID)
	assert.Equal(t, "T-004", out[2].GlobalID)
	assert.Equal(t, 1, report.RemovedCount)
}

// TestDeduplicateTasks_MultipleGroups verifies that multiple independent duplicate groups
// are all processed correctly.
func TestDeduplicateTasks_MultipleGroups(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Implement auth", AcceptanceCriteria: []string{"ac1"}},
		{GlobalID: "T-002", Title: "Create auth", AcceptanceCriteria: []string{"ac2"}}, // dup of T-001
		{GlobalID: "T-003", Title: "Build schema", AcceptanceCriteria: []string{"ac3"}},
		{GlobalID: "T-004", Title: "Design schema", AcceptanceCriteria: []string{"ac4"}}, // dup of T-003
		{GlobalID: "T-005", Title: "Write tests", AcceptanceCriteria: []string{"ac5"}},
	}

	out, report := DeduplicateTasks(tasks)

	require.Len(t, out, 3)
	assert.Equal(t, 5, report.OriginalCount)
	assert.Equal(t, 2, report.RemovedCount)
	assert.Equal(t, 3, report.FinalCount)
	assert.Len(t, report.Merges, 2)
}

// TestDeduplicateTasks_ReportCounts verifies that OriginalCount + RemovedCount and
// FinalCount are consistent.
func TestDeduplicateTasks_ReportCounts(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Implement service", AcceptanceCriteria: []string{"a"}},
		{GlobalID: "T-002", Title: "Create service", AcceptanceCriteria: []string{"b"}},
		{GlobalID: "T-003", Title: "Add service", AcceptanceCriteria: []string{"c"}},
	}

	out, report := DeduplicateTasks(tasks)

	require.Len(t, out, 1)
	assert.Equal(t, 3, report.OriginalCount)
	assert.Equal(t, 2, report.RemovedCount)
	assert.Equal(t, 1, report.FinalCount)
	assert.Equal(t, report.OriginalCount-report.RemovedCount, report.FinalCount)
}

// TestDeduplicateTasks_InputNotMutated verifies that the original tasks slice is not
// mutated by DeduplicateTasks.
func TestDeduplicateTasks_InputNotMutated(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		{GlobalID: "T-001", Title: "Implement auth", AcceptanceCriteria: []string{"ac1"}},
		{GlobalID: "T-002", Title: "Create auth", AcceptanceCriteria: []string{"ac2"}},
	}
	originalAC := make([]string, len(tasks[0].AcceptanceCriteria))
	copy(originalAC, tasks[0].AcceptanceCriteria)

	_, _ = DeduplicateTasks(tasks)

	// The original slice must not be mutated; T-001's AC must still be the original.
	assert.Equal(t, originalAC, tasks[0].AcceptanceCriteria,
		"input task AcceptanceCriteria must not be mutated")
}

// buildTasks is a test helper that creates n TaskDef instances with unique TempIDs
// derived from the provided epic prefix.
func buildTasks(t *testing.T, epicPrefix string, n int) []TaskDef {
	t.Helper()

	tasks := make([]TaskDef, n)
	for i := range tasks {
		tasks[i] = TaskDef{
			TempID:             fmt.Sprintf("%s-T%02d", epicPrefix, i+1),
			Title:              fmt.Sprintf("Task %d", i+1),
			Description:        fmt.Sprintf("Description for task %d", i+1),
			AcceptanceCriteria: []string{fmt.Sprintf("criterion %d", i+1)},
			Effort:             "small",
			Priority:           "must-have",
		}
	}
	return tasks
}

// --- ValidateDAG tests ---

// makeTask is a test helper that returns a MergedTask with the given GlobalID and deps.
func makeTask(id string, deps ...string) MergedTask {
	return MergedTask{
		GlobalID:     id,
		TempID:       id,
		Title:        "Task " + id,
		Description:  "D",
		Dependencies: deps,
	}
}

func TestValidateDAG_EmptyTasks(t *testing.T) {
	t.Parallel()

	v := ValidateDAG(nil)
	assert.True(t, v.Valid)
	assert.Empty(t, v.Errors)
	assert.Empty(t, v.TopologicalOrder)
}

func TestValidateDAG_SingleTask_NoDeps(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{makeTask("T-001")}
	v := ValidateDAG(tasks)

	assert.True(t, v.Valid)
	assert.Empty(t, v.Errors)
	assert.Equal(t, []string{"T-001"}, v.TopologicalOrder)
	assert.Equal(t, 0, v.Depths["T-001"])
	assert.Equal(t, 0, v.MaxDepth)
}

func TestValidateDAG_LinearChain_TopologicalOrder(t *testing.T) {
	t.Parallel()

	// T-001 <- T-002 <- T-003
	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002", "T-001"),
		makeTask("T-003", "T-002"),
	}
	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Equal(t, []string{"T-001", "T-002", "T-003"}, v.TopologicalOrder)
	assert.Equal(t, 0, v.Depths["T-001"])
	assert.Equal(t, 1, v.Depths["T-002"])
	assert.Equal(t, 2, v.Depths["T-003"])
	assert.Equal(t, 2, v.MaxDepth)
}

func TestValidateDAG_Diamond_DepthIsLongestPath(t *testing.T) {
	t.Parallel()

	//   T-001
	//  /     \
	// T-002  T-003
	//  \     /
	//   T-004
	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002", "T-001"),
		makeTask("T-003", "T-001"),
		makeTask("T-004", "T-002", "T-003"),
	}
	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Equal(t, 0, v.Depths["T-001"])
	assert.Equal(t, 1, v.Depths["T-002"])
	assert.Equal(t, 1, v.Depths["T-003"])
	// T-004 has longest path 2 (via T-001 -> T-002 -> T-004 or T-001 -> T-003 -> T-004)
	assert.Equal(t, 2, v.Depths["T-004"])
	assert.Equal(t, 2, v.MaxDepth)
}

func TestValidateDAG_Deterministic_TopologicalOrder(t *testing.T) {
	t.Parallel()

	// All tasks are independent roots; they should be returned in sorted ID order.
	tasks := []MergedTask{
		makeTask("T-003"),
		makeTask("T-001"),
		makeTask("T-002"),
	}
	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Equal(t, []string{"T-001", "T-002", "T-003"}, v.TopologicalOrder)
}

func TestValidateDAG_DanglingReference(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001", "T-999"), // T-999 does not exist
	}
	v := ValidateDAG(tasks)

	assert.False(t, v.Valid)
	require.Len(t, v.Errors, 1)
	assert.Equal(t, DanglingReference, v.Errors[0].Type)
	assert.Equal(t, "T-001", v.Errors[0].TaskID)
	assert.Contains(t, v.Errors[0].Details, "T-001")
	assert.Contains(t, v.Errors[0].Details, "T-999")
	assert.Contains(t, v.Errors[0].Details, "does not exist")
}

func TestValidateDAG_SelfReference(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001", "T-001"), // depends on itself
	}
	v := ValidateDAG(tasks)

	assert.False(t, v.Valid)
	require.Len(t, v.Errors, 1)
	assert.Equal(t, SelfReference, v.Errors[0].Type)
	assert.Equal(t, "T-001", v.Errors[0].TaskID)
	assert.Contains(t, v.Errors[0].Details, "depends on itself")
}

func TestValidateDAG_SelfReferenceDoesNotCauseCycle(t *testing.T) {
	t.Parallel()

	// T-001 has a self-ref and T-002 depends on T-001. The self-ref is an error but
	// the rest of the graph (T-001 -> T-002) should not be considered a cycle.
	tasks := []MergedTask{
		makeTask("T-001", "T-001"),
		makeTask("T-002", "T-001"),
	}
	v := ValidateDAG(tasks)

	// Only the self-reference error should be reported (no cycle).
	assert.False(t, v.Valid)
	assert.Len(t, v.Errors, 1)
	assert.Equal(t, SelfReference, v.Errors[0].Type)
}

func TestValidateDAG_SimpleCycle_TwoNodes(t *testing.T) {
	t.Parallel()

	// T-001 depends on T-002, T-002 depends on T-001.
	tasks := []MergedTask{
		makeTask("T-001", "T-002"),
		makeTask("T-002", "T-001"),
	}
	v := ValidateDAG(tasks)

	assert.False(t, v.Valid)
	require.Len(t, v.Errors, 1)
	assert.Equal(t, CycleDetected, v.Errors[0].Type)
	assert.Contains(t, v.Errors[0].Details, "T-001")
	assert.Contains(t, v.Errors[0].Details, "T-002")
	assert.Len(t, v.Errors[0].Cycle, 2)
}

func TestValidateDAG_SimpleCycle_ThreeNodes(t *testing.T) {
	t.Parallel()

	// T-001 -> T-002 -> T-003 -> T-001
	tasks := []MergedTask{
		makeTask("T-001", "T-003"),
		makeTask("T-002", "T-001"),
		makeTask("T-003", "T-002"),
	}
	v := ValidateDAG(tasks)

	assert.False(t, v.Valid)
	require.Len(t, v.Errors, 1)
	assert.Equal(t, CycleDetected, v.Errors[0].Type)
	assert.Len(t, v.Errors[0].Cycle, 3)
	assert.Contains(t, v.Errors[0].Details, "cycle detected involving tasks")
}

func TestValidateDAG_PartialCycle_RootTasksOrdered(t *testing.T) {
	t.Parallel()

	// T-001 is a valid root; T-002 and T-003 form a cycle.
	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002", "T-003"),
		makeTask("T-003", "T-002"),
	}
	v := ValidateDAG(tasks)

	// Graph is invalid because of the cycle.
	assert.False(t, v.Valid)
	require.Len(t, v.Errors, 1)
	assert.Equal(t, CycleDetected, v.Errors[0].Type)
	assert.ElementsMatch(t, []string{"T-002", "T-003"}, v.Errors[0].Cycle)
}

func TestValidateDAG_MultipleDanglingRefs_AllReported(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001", "T-888", "T-999"),
	}
	v := ValidateDAG(tasks)

	assert.False(t, v.Valid)
	assert.Len(t, v.Errors, 2)
	for _, e := range v.Errors {
		assert.Equal(t, DanglingReference, e.Type)
	}
}

func TestValidateDAG_MixedErrors_AllReported(t *testing.T) {
	t.Parallel()

	// T-001 has a self-ref, T-002 has a dangling ref, T-003 and T-004 form a cycle.
	tasks := []MergedTask{
		makeTask("T-001", "T-001"), // self-reference
		makeTask("T-002", "T-999"), // dangling reference
		makeTask("T-003", "T-004"), // part of cycle
		makeTask("T-004", "T-003"), // part of cycle
	}
	v := ValidateDAG(tasks)

	assert.False(t, v.Valid)

	typeSet := make(map[DAGErrorType]int)
	for _, e := range v.Errors {
		typeSet[e.Type]++
	}
	assert.Equal(t, 1, typeSet[SelfReference])
	assert.Equal(t, 1, typeSet[DanglingReference])
	assert.Equal(t, 1, typeSet[CycleDetected])
}

func TestValidateDAG_GraphTooLarge(t *testing.T) {
	t.Parallel()

	// Build a slice just over the limit.
	tasks := make([]MergedTask, maxDAGTasks+1)
	for i := range tasks {
		tasks[i] = makeTask(fmt.Sprintf("T-%04d", i+1))
	}

	v := ValidateDAG(tasks)
	assert.False(t, v.Valid)
	require.Len(t, v.Errors, 1)
	assert.Contains(t, v.Errors[0].Details, "graph too large")
}

func TestValidateDAG_DanglingRefDoesNotInfluenceCycleDetection(t *testing.T) {
	t.Parallel()

	// T-001 has a dangling dep AND T-002/T-003 form a valid chain.
	// Neither T-002 nor T-003 should be reported as a cycle.
	tasks := []MergedTask{
		makeTask("T-001", "T-888"),
		makeTask("T-002"),
		makeTask("T-003", "T-002"),
	}
	v := ValidateDAG(tasks)

	// Only 1 error: dangling reference on T-001.
	assert.False(t, v.Valid)
	require.Len(t, v.Errors, 1)
	assert.Equal(t, DanglingReference, v.Errors[0].Type)
}

func TestValidateDAG_LongestPathDepth_NotShortestPath(t *testing.T) {
	t.Parallel()

	// Graph:
	//   T-001 (depth 0)
	//   T-002 (depth 0)
	//   T-003 -> T-001 (depth 1)
	//   T-004 -> T-001, T-002, T-003 (depth = max(0,0,1)+1 = 2)
	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002"),
		makeTask("T-003", "T-001"),
		makeTask("T-004", "T-001", "T-002", "T-003"),
	}
	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Equal(t, 0, v.Depths["T-001"])
	assert.Equal(t, 0, v.Depths["T-002"])
	assert.Equal(t, 1, v.Depths["T-003"])
	assert.Equal(t, 2, v.Depths["T-004"]) // longest path: T-001 -> T-003 -> T-004
	assert.Equal(t, 2, v.MaxDepth)
}

func TestValidateDAG_AllErrors_NilTopologicalOrder(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001", "T-002"),
		makeTask("T-002", "T-001"),
	}
	v := ValidateDAG(tasks)

	assert.False(t, v.Valid)
	assert.Nil(t, v.TopologicalOrder)
	assert.Nil(t, v.Depths)
}

func TestValidateDAG_MultipleIndependentCycles(t *testing.T) {
	t.Parallel()

	// Two independent two-node cycles: (T-001, T-002) and (T-003, T-004).
	tasks := []MergedTask{
		makeTask("T-001", "T-002"),
		makeTask("T-002", "T-001"),
		makeTask("T-003", "T-004"),
		makeTask("T-004", "T-003"),
	}
	v := ValidateDAG(tasks)

	assert.False(t, v.Valid)
	cycleErrors := 0
	for _, e := range v.Errors {
		if e.Type == CycleDetected {
			cycleErrors++
		}
	}
	// Both cycles must be reported.
	assert.Equal(t, 2, cycleErrors)
}

// --- TopologicalDepths tests ---

func TestTopologicalDepths_NilInput(t *testing.T) {
	t.Parallel()

	result := TopologicalDepths(nil)
	assert.Nil(t, result)
}

func TestTopologicalDepths_LinearChain(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002", "T-001"),
		makeTask("T-003", "T-002"),
	}
	depths := TopologicalDepths(tasks)

	require.NotNil(t, depths)
	assert.Equal(t, 0, depths["T-001"])
	assert.Equal(t, 1, depths["T-002"])
	assert.Equal(t, 2, depths["T-003"])
}

func TestTopologicalDepths_InvalidDAG_ReturnsNil(t *testing.T) {
	t.Parallel()

	// Cycle makes the DAG invalid; TopologicalDepths must return nil.
	tasks := []MergedTask{
		makeTask("T-001", "T-002"),
		makeTask("T-002", "T-001"),
	}
	depths := TopologicalDepths(tasks)

	assert.Nil(t, depths)
}

func TestTopologicalDepths_IndependentTasks_AllDepthZero(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002"),
		makeTask("T-003"),
	}
	depths := TopologicalDepths(tasks)

	require.NotNil(t, depths)
	assert.Equal(t, 0, depths["T-001"])
	assert.Equal(t, 0, depths["T-002"])
	assert.Equal(t, 0, depths["T-003"])
}

// makeMergedTask is a test helper that returns a MergedTask with the given GlobalID,
// EpicID "E-001", and the provided dependency IDs. It mirrors the makeTask helper but
// includes a consistent EpicID and TempID prefix to match the task spec requirements.
func makeMergedTask(id string, deps ...string) MergedTask {
	return MergedTask{
		GlobalID:     id,
		TempID:       "temp-" + id,
		EpicID:       "E-001",
		Title:        "Task " + id,
		Dependencies: deps,
	}
}

// --- Additional ValidateDAG tests for T-063 ---

// TestValidateDAG_TableDriven exercises the primary ValidateDAG acceptance criteria
// in a single table-driven test, covering valid and invalid graphs.
func TestValidateDAG_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		tasks         []MergedTask
		wantValid     bool
		wantErrTypes  []DAGErrorType
		wantDepths    map[string]int
		wantMaxDepth  int
		wantTopoFirst string // first element in topological order (optional check)
		wantTopoLast  string // last element in topological order (optional check)
	}{
		{
			name: "linear chain A->B->C valid depth",
			tasks: []MergedTask{
				makeTask("T-001"),
				makeTask("T-002", "T-001"),
				makeTask("T-003", "T-002"),
			},
			wantValid:     true,
			wantDepths:    map[string]int{"T-001": 0, "T-002": 1, "T-003": 2},
			wantMaxDepth:  2,
			wantTopoFirst: "T-001",
			wantTopoLast:  "T-003",
		},
		{
			name: "diamond depth is longest path not shortest",
			tasks: []MergedTask{
				makeTask("T-001"),
				makeTask("T-002", "T-001"),
				makeTask("T-003", "T-001"),
				makeTask("T-004", "T-002", "T-003"),
			},
			wantValid:    true,
			wantDepths:   map[string]int{"T-001": 0, "T-002": 1, "T-003": 1, "T-004": 2},
			wantMaxDepth: 2,
			wantTopoLast: "T-004",
		},
		{
			name: "all tasks independent depth zero",
			tasks: []MergedTask{
				makeTask("T-001"),
				makeTask("T-002"),
				makeTask("T-003"),
			},
			wantValid:    true,
			wantDepths:   map[string]int{"T-001": 0, "T-002": 0, "T-003": 0},
			wantMaxDepth: 0,
		},
		{
			name: "self-reference is SelfReference not CycleDetected",
			tasks: []MergedTask{
				makeTask("T-001", "T-001"),
			},
			wantValid:    false,
			wantErrTypes: []DAGErrorType{SelfReference},
		},
		{
			name: "dangling reference reports both IDs",
			tasks: []MergedTask{
				makeTask("T-001", "T-999"),
			},
			wantValid:    false,
			wantErrTypes: []DAGErrorType{DanglingReference},
		},
		{
			name: "two-node cycle is CycleDetected",
			tasks: []MergedTask{
				makeTask("T-001", "T-002"),
				makeTask("T-002", "T-001"),
			},
			wantValid:    false,
			wantErrTypes: []DAGErrorType{CycleDetected},
		},
		{
			name: "three-node cycle is CycleDetected",
			tasks: []MergedTask{
				makeTask("T-001", "T-003"),
				makeTask("T-002", "T-001"),
				makeTask("T-003", "T-002"),
			},
			wantValid:    false,
			wantErrTypes: []DAGErrorType{CycleDetected},
		},
		{
			name: "self-ref plus dangling ref both reported",
			tasks: []MergedTask{
				makeTask("T-001", "T-001"),
				makeTask("T-002", "T-888"),
			},
			wantValid:    false,
			wantErrTypes: []DAGErrorType{SelfReference, DanglingReference},
		},
		{
			name:      "empty task list is valid",
			tasks:     []MergedTask{},
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := ValidateDAG(tt.tasks)

			assert.Equal(t, tt.wantValid, v.Valid, "Valid mismatch")

			if !tt.wantValid {
				assert.NotEmpty(t, v.Errors, "expected errors but got none")
				if len(tt.wantErrTypes) > 0 {
					gotTypes := make(map[DAGErrorType]bool, len(v.Errors))
					for _, e := range v.Errors {
						gotTypes[e.Type] = true
					}
					for _, wantType := range tt.wantErrTypes {
						assert.True(t, gotTypes[wantType], "expected error type %d not found in %v", wantType, v.Errors)
					}
				}
				return
			}

			// Valid graph checks.
			if tt.wantDepths != nil {
				require.NotNil(t, v.Depths)
				for id, wantDepth := range tt.wantDepths {
					assert.Equal(t, wantDepth, v.Depths[id], "depth mismatch for %s", id)
				}
			}
			assert.Equal(t, tt.wantMaxDepth, v.MaxDepth)

			if tt.wantTopoFirst != "" {
				require.NotEmpty(t, v.TopologicalOrder)
				assert.Equal(t, tt.wantTopoFirst, v.TopologicalOrder[0], "expected topological first")
			}
			if tt.wantTopoLast != "" {
				require.NotEmpty(t, v.TopologicalOrder)
				assert.Equal(t, tt.wantTopoLast, v.TopologicalOrder[len(v.TopologicalOrder)-1], "expected topological last")
			}
		})
	}
}

// TestValidateDAG_LargeValidDAG verifies that a large DAG (50+ tasks in a linear chain)
// validates correctly and computes depths without errors.
func TestValidateDAG_LargeValidDAG(t *testing.T) {
	t.Parallel()

	const n = 60
	tasks := make([]MergedTask, n)
	tasks[0] = makeTask("T-001")
	for i := 1; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		prevID := fmt.Sprintf("T-%03d", i)
		tasks[i] = makeTask(id, prevID)
	}

	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Empty(t, v.Errors)
	assert.Len(t, v.TopologicalOrder, n)
	assert.Equal(t, n-1, v.MaxDepth)

	// Verify every task has the correct depth (linear chain: depth == position).
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		assert.Equal(t, i, v.Depths[id], "depth mismatch for task %s", id)
	}
}

// TestValidateDAG_LargeDAG_50Tasks verifies the explicit 50-task requirement from the spec.
func TestValidateDAG_LargeDAG_50Tasks(t *testing.T) {
	t.Parallel()

	const n = 50
	tasks := make([]MergedTask, n)
	tasks[0] = makeTask("T-001")
	for i := 1; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		prevID := fmt.Sprintf("T-%03d", i)
		tasks[i] = makeTask(id, prevID)
	}

	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Len(t, v.TopologicalOrder, n)
	assert.Equal(t, n-1, v.MaxDepth)

	// Root has depth 0, last task has depth n-1.
	assert.Equal(t, 0, v.Depths["T-001"])
	assert.Equal(t, n-1, v.Depths[fmt.Sprintf("T-%03d", n)])
}

// TestValidateDAG_DisconnectedComponents verifies that two separate chains are both
// validated correctly with depths computed independently.
func TestValidateDAG_DisconnectedComponents(t *testing.T) {
	t.Parallel()

	// Chain 1: T-001 -> T-002 -> T-003
	// Chain 2: T-004 -> T-005
	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002", "T-001"),
		makeTask("T-003", "T-002"),
		makeTask("T-004"),
		makeTask("T-005", "T-004"),
	}

	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Empty(t, v.Errors)
	assert.Len(t, v.TopologicalOrder, 5)

	// Chain 1 depths.
	assert.Equal(t, 0, v.Depths["T-001"])
	assert.Equal(t, 1, v.Depths["T-002"])
	assert.Equal(t, 2, v.Depths["T-003"])

	// Chain 2 depths (independent; start from 0 again).
	assert.Equal(t, 0, v.Depths["T-004"])
	assert.Equal(t, 1, v.Depths["T-005"])

	// MaxDepth comes from the longer chain.
	assert.Equal(t, 2, v.MaxDepth)
}

// TestValidateDAG_DisconnectedComponents_EqualChains verifies two chains of equal
// length are both processed and max depth reflects the shared longest path.
func TestValidateDAG_DisconnectedComponents_EqualChains(t *testing.T) {
	t.Parallel()

	// Two independent chains each of length 3.
	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002", "T-001"),
		makeTask("T-003", "T-002"),
		makeTask("T-004"),
		makeTask("T-005", "T-004"),
		makeTask("T-006", "T-005"),
	}

	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Len(t, v.TopologicalOrder, 6)
	assert.Equal(t, 2, v.MaxDepth)
	assert.Equal(t, 2, v.Depths["T-003"])
	assert.Equal(t, 2, v.Depths["T-006"])
}

// TestValidateDAG_EmptyDependencyString verifies that a task with an empty string in its
// Dependencies slice is treated as a dangling reference (the empty-string task ID does not exist).
func TestValidateDAG_EmptyDependencyString(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001", ""), // "" is not a valid task ID
	}

	v := ValidateDAG(tasks)

	// The empty string is not in the task set, so it is a dangling reference.
	assert.False(t, v.Valid)
	require.Len(t, v.Errors, 1)
	assert.Equal(t, DanglingReference, v.Errors[0].Type)
	assert.Equal(t, "T-001", v.Errors[0].TaskID)
}

// TestValidateDAG_VeryDeepGraph verifies that a graph with depth > 100 does not panic
// or overflow (the implementation uses BFS/Kahn, not recursive DFS for cycle detection).
func TestValidateDAG_VeryDeepGraph(t *testing.T) {
	t.Parallel()

	const depth = 150
	tasks := make([]MergedTask, depth)
	// Build a linear chain: T-0001 -> T-0002 -> ... -> T-0150
	for i := 0; i < depth; i++ {
		id := fmt.Sprintf("T-%04d", i+1)
		if i == 0 {
			tasks[i] = makeTask(id)
		} else {
			prevID := fmt.Sprintf("T-%04d", i)
			tasks[i] = makeTask(id, prevID)
		}
	}

	v := ValidateDAG(tasks)

	require.True(t, v.Valid, "expected valid DAG, got errors: %v", v.Errors)
	assert.Equal(t, depth-1, v.MaxDepth)
	assert.Len(t, v.TopologicalOrder, depth)
	// Root has depth 0, leaf has depth depth-1.
	assert.Equal(t, 0, v.Depths["T-0001"])
	assert.Equal(t, depth-1, v.Depths[fmt.Sprintf("T-%04d", depth)])
}

// TestValidateDAG_MultipleRoots verifies that a graph with many independent root tasks
// (no dependencies) is valid and all roots have depth 0.
func TestValidateDAG_MultipleRoots(t *testing.T) {
	t.Parallel()

	const numRoots = 10
	tasks := make([]MergedTask, numRoots+1)
	rootIDs := make([]string, numRoots)
	for i := 0; i < numRoots; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		tasks[i] = makeTask(id)
		rootIDs[i] = id
	}
	// A single sink task that depends on all roots.
	sinkID := fmt.Sprintf("T-%03d", numRoots+1)
	tasks[numRoots] = makeTask(sinkID, rootIDs...)

	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Len(t, v.TopologicalOrder, numRoots+1)

	for _, rootID := range rootIDs {
		assert.Equal(t, 0, v.Depths[rootID], "root %s should have depth 0", rootID)
	}
	assert.Equal(t, 1, v.Depths[sinkID], "sink depth should be 1 (max of all roots + 1)")
	assert.Equal(t, 1, v.MaxDepth)
}

// TestValidateDAG_SingleSink verifies that a graph where all paths converge to one final
// task is valid and the sink has the correct depth.
func TestValidateDAG_SingleSink(t *testing.T) {
	t.Parallel()

	// T-001 -> T-004 (depth 1)
	// T-002 -> T-003 -> T-004 (depth 2)
	// T-004 is the single sink.
	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002"),
		makeTask("T-003", "T-002"),
		makeTask("T-004", "T-001", "T-003"),
	}

	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Equal(t, 0, v.Depths["T-001"])
	assert.Equal(t, 0, v.Depths["T-002"])
	assert.Equal(t, 1, v.Depths["T-003"])
	// T-004 longest path: T-002->T-003->T-004 = depth 2
	assert.Equal(t, 2, v.Depths["T-004"])
	assert.Equal(t, 2, v.MaxDepth)
}

// TestValidateDAG_MultipleRootsNoSink verifies a fan-out graph (one root, many leaves)
// is valid and all leaves have depth 1.
func TestValidateDAG_MultipleRootsNoSink(t *testing.T) {
	t.Parallel()

	// T-001 is the single root; T-002 through T-006 are all leaves depending on T-001.
	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002", "T-001"),
		makeTask("T-003", "T-001"),
		makeTask("T-004", "T-001"),
		makeTask("T-005", "T-001"),
		makeTask("T-006", "T-001"),
	}

	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Equal(t, "T-001", v.TopologicalOrder[0])
	assert.Equal(t, 0, v.Depths["T-001"])
	for _, id := range []string{"T-002", "T-003", "T-004", "T-005", "T-006"} {
		assert.Equal(t, 1, v.Depths[id], "leaf %s should have depth 1", id)
	}
	assert.Equal(t, 1, v.MaxDepth)
}

// TestValidateDAG_DeterministicAcrossMultipleRuns verifies that calling ValidateDAG
// many times on the same input always produces the exact same topological order.
// This guards against map-iteration non-determinism leaking into the output.
func TestValidateDAG_DeterministicAcrossMultipleRuns(t *testing.T) {
	t.Parallel()

	// Mix of roots and dependent tasks.
	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002"),
		makeTask("T-003"),
		makeTask("T-004", "T-001"),
		makeTask("T-005", "T-002"),
		makeTask("T-006", "T-003"),
		makeTask("T-007", "T-004", "T-005"),
		makeTask("T-008", "T-006"),
	}

	const iterations = 20
	first := ValidateDAG(tasks)
	require.True(t, first.Valid)

	for i := 1; i < iterations; i++ {
		got := ValidateDAG(tasks)
		require.True(t, got.Valid, "iteration %d: expected valid", i)
		assert.Equal(t, first.TopologicalOrder, got.TopologicalOrder,
			"iteration %d produced different topological order", i)
		assert.Equal(t, first.Depths, got.Depths,
			"iteration %d produced different depths", i)
	}
}

// TestValidateDAG_SelfRefAndDanglingBothReported verifies the "multiple independent errors"
// requirement: a task with a self-reference AND a task with a dangling reference each produce
// their own distinct errors in a single ValidateDAG call.
func TestValidateDAG_SelfRefAndDanglingBothReported(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001", "T-001"), // self-reference
		makeTask("T-002", "T-999"), // dangling reference
		makeTask("T-003"),          // valid
	}

	v := ValidateDAG(tasks)

	assert.False(t, v.Valid)

	// Count error types.
	selfRefCount := 0
	danglingCount := 0
	for _, e := range v.Errors {
		switch e.Type {
		case SelfReference:
			selfRefCount++
			assert.Equal(t, "T-001", e.TaskID)
			assert.Contains(t, e.Details, "T-001")
		case DanglingReference:
			danglingCount++
			assert.Equal(t, "T-002", e.TaskID)
			assert.Contains(t, e.Details, "T-002")
			assert.Contains(t, e.Details, "T-999")
			assert.Contains(t, e.Details, "does not exist")
		default:
		}
	}
	assert.Equal(t, 1, selfRefCount, "expected exactly one SelfReference error")
	assert.Equal(t, 1, danglingCount, "expected exactly one DanglingReference error")
}

// TestValidateDAG_DanglingReferenceErrorMessage verifies the exact error-message format
// of a dangling reference: it must mention both the referencing task and the missing ID.
func TestValidateDAG_DanglingReferenceErrorMessage(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001", "T-ZZZ"),
	}

	v := ValidateDAG(tasks)

	require.False(t, v.Valid)
	require.Len(t, v.Errors, 1)

	e := v.Errors[0]
	assert.Equal(t, DanglingReference, e.Type)
	assert.Equal(t, "T-001", e.TaskID)
	assert.Contains(t, e.Details, "T-001", "details must mention the referencing task")
	assert.Contains(t, e.Details, "T-ZZZ", "details must mention the missing dependency")
	assert.Contains(t, e.Details, "does not exist")
}

// TestValidateDAG_SelfReferenceErrorMessage verifies the exact error-message format
// of a self-reference error.
func TestValidateDAG_SelfReferenceErrorMessage(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-042", "T-042"),
	}

	v := ValidateDAG(tasks)

	require.False(t, v.Valid)
	require.Len(t, v.Errors, 1)

	e := v.Errors[0]
	assert.Equal(t, SelfReference, e.Type)
	assert.Equal(t, "T-042", e.TaskID)
	assert.Contains(t, e.Details, "T-042")
	assert.Contains(t, e.Details, "depends on itself")
}

// TestValidateDAG_CycleErrorContainsAllCycleNodes verifies that when a cycle is detected,
// the Cycle field in the DAGError contains all node IDs forming the cycle.
func TestValidateDAG_CycleErrorContainsAllCycleNodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		tasks         []MergedTask
		expectedCycle []string // IDs that must all appear in Cycle
	}{
		{
			name: "two-node cycle",
			tasks: []MergedTask{
				makeTask("T-001", "T-002"),
				makeTask("T-002", "T-001"),
			},
			expectedCycle: []string{"T-001", "T-002"},
		},
		{
			name: "three-node cycle",
			tasks: []MergedTask{
				makeTask("T-001", "T-003"),
				makeTask("T-002", "T-001"),
				makeTask("T-003", "T-002"),
			},
			expectedCycle: []string{"T-001", "T-002", "T-003"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := ValidateDAG(tt.tasks)

			assert.False(t, v.Valid)
			require.NotEmpty(t, v.Errors)

			// Find the CycleDetected error.
			var cycleErr *DAGError
			for i := range v.Errors {
				if v.Errors[i].Type == CycleDetected {
					cycleErr = &v.Errors[i]
					break
				}
			}
			require.NotNil(t, cycleErr, "expected a CycleDetected error")

			// All expected nodes must appear in the cycle.
			for _, id := range tt.expectedCycle {
				assert.Contains(t, cycleErr.Cycle, id, "cycle must contain %s", id)
			}
			assert.Contains(t, cycleErr.Details, "cycle detected involving tasks")
		})
	}
}

// TestValidateDAG_NilTasksVsEmptyTasksEquivalent verifies that nil and empty slices
// produce the same result: a valid empty DAG.
func TestValidateDAG_NilTasksVsEmptyTasksEquivalent(t *testing.T) {
	t.Parallel()

	vNil := ValidateDAG(nil)
	vEmpty := ValidateDAG([]MergedTask{})

	assert.Equal(t, vNil.Valid, vEmpty.Valid)
	assert.Equal(t, len(vNil.Errors), len(vEmpty.Errors))
	assert.Empty(t, vNil.TopologicalOrder)
	assert.Empty(t, vEmpty.TopologicalOrder)
}

// TestValidateDAG_MaxDepthReflectsDeepestTask verifies that MaxDepth always matches
// the highest depth value in the Depths map.
func TestValidateDAG_MaxDepthReflectsDeepestTask(t *testing.T) {
	t.Parallel()

	// T-005 is the deepest: T-001->T-002->T-003->T-004->T-005 (depth 4).
	// T-006 branches from T-001 (depth 1), so it does not change MaxDepth.
	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002", "T-001"),
		makeTask("T-003", "T-002"),
		makeTask("T-004", "T-003"),
		makeTask("T-005", "T-004"),
		makeTask("T-006", "T-001"),
	}

	v := ValidateDAG(tasks)

	require.True(t, v.Valid)

	// Find maximum in Depths map.
	maxFromMap := 0
	for _, d := range v.Depths {
		if d > maxFromMap {
			maxFromMap = d
		}
	}
	assert.Equal(t, maxFromMap, v.MaxDepth, "MaxDepth must equal the highest value in Depths")
	assert.Equal(t, 4, v.MaxDepth)
}

// TestValidateDAG_ValidGraphHasNoNilFields verifies that a valid DAG always has
// non-nil TopologicalOrder and Depths (even for edge cases like a single task).
func TestValidateDAG_ValidGraphHasNoNilFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		tasks []MergedTask
	}{
		{name: "single task", tasks: []MergedTask{makeTask("T-001")}},
		{name: "two tasks no dep", tasks: []MergedTask{makeTask("T-001"), makeTask("T-002")}},
		{name: "two tasks with dep", tasks: []MergedTask{makeTask("T-001"), makeTask("T-002", "T-001")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := ValidateDAG(tt.tasks)

			require.True(t, v.Valid)
			assert.NotNil(t, v.TopologicalOrder)
			assert.NotNil(t, v.Depths)
		})
	}
}

// TestValidateDAG_TopoOrderLength verifies the topological order has exactly
// the same number of tasks as the input.
func TestValidateDAG_TopoOrderLength(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002", "T-001"),
		makeTask("T-003"),
		makeTask("T-004", "T-003"),
		makeTask("T-005", "T-002", "T-004"),
	}

	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Len(t, v.TopologicalOrder, len(tasks))
	// T-005 must be last (depends on T-002 and T-004).
	assert.Equal(t, "T-005", v.TopologicalOrder[len(v.TopologicalOrder)-1])
}

// TestValidateDAG_DepthZeroForAllRoots verifies that every task with no dependencies
// receives depth 0, regardless of graph structure.
func TestValidateDAG_DepthZeroForAllRoots(t *testing.T) {
	t.Parallel()

	// Multiple independent roots plus one convergence task.
	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002"),
		makeTask("T-003"),
		makeTask("T-004"),
		makeTask("T-005", "T-001", "T-002", "T-003", "T-004"),
	}

	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	for _, id := range []string{"T-001", "T-002", "T-003", "T-004"} {
		assert.Equal(t, 0, v.Depths[id], "root %s should have depth 0", id)
	}
	assert.Equal(t, 1, v.Depths["T-005"])
}

// TestValidateDAG_MakeMergedTaskHelper verifies that makeMergedTask produces correctly
// structured tasks that are accepted by ValidateDAG.
func TestValidateDAG_MakeMergedTaskHelper(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeMergedTask("T-001"),
		makeMergedTask("T-002", "T-001"),
	}

	v := ValidateDAG(tasks)

	require.True(t, v.Valid)
	assert.Equal(t, []string{"T-001", "T-002"}, v.TopologicalOrder)
	assert.Equal(t, 0, v.Depths["T-001"])
	assert.Equal(t, 1, v.Depths["T-002"])

	// Verify helper fields.
	assert.Equal(t, "temp-T-001", tasks[0].TempID)
	assert.Equal(t, "E-001", tasks[0].EpicID)
	assert.Equal(t, "Task T-001", tasks[0].Title)
}

// --- Additional TopologicalDepths tests ---

// TestTopologicalDepths_EmptySlice verifies that an empty (non-nil) slice returns nil.
func TestTopologicalDepths_EmptySlice(t *testing.T) {
	t.Parallel()

	result := TopologicalDepths([]MergedTask{})
	assert.Nil(t, result)
}

// TestTopologicalDepths_ValidDAG_AllDepths verifies that TopologicalDepths returns
// the same depths as ValidateDAG.Depths for a non-trivial graph.
func TestTopologicalDepths_ValidDAG_AllDepths(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002", "T-001"),
		makeTask("T-003", "T-001"),
		makeTask("T-004", "T-002", "T-003"),
	}

	depths := TopologicalDepths(tasks)
	v := ValidateDAG(tasks)

	require.NotNil(t, depths)
	assert.Equal(t, v.Depths, depths)
}

// TestTopologicalDepths_DanglingRef_ReturnsNil verifies that TopologicalDepths returns nil
// when the graph has a dangling reference (which makes it invalid).
func TestTopologicalDepths_DanglingRef_ReturnsNil(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001", "T-999"),
	}

	result := TopologicalDepths(tasks)
	assert.Nil(t, result)
}

// TestTopologicalDepths_SelfRef_ReturnsNil verifies that TopologicalDepths returns nil
// when the graph has a self-reference.
func TestTopologicalDepths_SelfRef_ReturnsNil(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001", "T-001"),
	}

	result := TopologicalDepths(tasks)
	assert.Nil(t, result)
}

// TestTopologicalDepths_DiamondGraph verifies correct longest-path depths
// for a diamond-shaped graph.
func TestTopologicalDepths_DiamondGraph(t *testing.T) {
	t.Parallel()

	tasks := []MergedTask{
		makeTask("T-001"),
		makeTask("T-002", "T-001"),
		makeTask("T-003", "T-001"),
		makeTask("T-004", "T-002", "T-003"),
	}

	depths := TopologicalDepths(tasks)

	require.NotNil(t, depths)
	assert.Equal(t, 0, depths["T-001"])
	assert.Equal(t, 1, depths["T-002"])
	assert.Equal(t, 1, depths["T-003"])
	assert.Equal(t, 2, depths["T-004"])
}

// TestTopologicalDepths_LargeGraph verifies that TopologicalDepths handles a larger
// graph (50 tasks) without error.
func TestTopologicalDepths_LargeGraph(t *testing.T) {
	t.Parallel()

	const n = 50
	tasks := make([]MergedTask, n)
	tasks[0] = makeTask("T-001")
	for i := 1; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		prevID := fmt.Sprintf("T-%03d", i)
		tasks[i] = makeTask(id, prevID)
	}

	depths := TopologicalDepths(tasks)

	require.NotNil(t, depths)
	assert.Equal(t, 0, depths["T-001"])
	assert.Equal(t, n-1, depths[fmt.Sprintf("T-%03d", n)])
}
