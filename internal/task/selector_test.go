package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Test helpers -----------------------------------------------------------

// makeSpec builds a minimal ParsedTaskSpec for test use.
func makeSpec(id string, deps []string) *ParsedTaskSpec {
	if deps == nil {
		deps = []string{}
	}
	return &ParsedTaskSpec{
		ID:           id,
		Title:        "Task " + id,
		Dependencies: deps,
	}
}

// selectorPhases returns a standard 2-phase slice used across selector tests.
// Phase 1: T-001 to T-005
// Phase 2: T-006 to T-010
func selectorPhases() []Phase {
	return []Phase{
		{ID: 1, Name: "Phase One", StartTask: "T-001", EndTask: "T-005"},
		{ID: 2, Name: "Phase Two", StartTask: "T-006", EndTask: "T-010"},
	}
}

// writeStateContent writes a state file to a temp dir and returns a StateManager.
func writeStateContent(t *testing.T, lines []string) *StateManager {
	t.Helper()
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return NewStateManager(path)
}

// emptyStateManager returns a StateManager pointing at a non-existent file
// (so all queries return empty / not_started).
func emptyStateManager(t *testing.T) *StateManager {
	t.Helper()
	return NewStateManager(filepath.Join(t.TempDir(), "nonexistent.conf"))
}

// ---- NewTaskSelector --------------------------------------------------------

func TestNewTaskSelector_BuildsSpecMap(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	require.NotNil(t, sel)
	assert.Len(t, sel.specMap, 2)
	assert.Contains(t, sel.specMap, "T-001")
	assert.Contains(t, sel.specMap, "T-002")
}

func TestNewTaskSelector_EmptySpecs(t *testing.T) {
	t.Parallel()

	sel := NewTaskSelector(nil, emptyStateManager(t), selectorPhases())
	require.NotNil(t, sel)
	assert.Empty(t, sel.specMap)
}

// ---- SelectNext -------------------------------------------------------------

func TestSelectNext_SelectsFirstNotStarted(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
		makeSpec("T-003", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|claude||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-002", got.ID)
}

func TestSelectNext_SkipsBlockedTask(t *testing.T) {
	t.Parallel()

	// T-001 and T-002 are not_started; T-002 depends on T-003 which is not yet
	// completed. So T-001 (no deps) should be selected.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", []string{"T-003"}),
		makeSpec("T-003", nil),
	}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-001", got.ID)
}

func TestSelectNext_ReturnsNilWhenAllDone(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|completed|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	assert.Nil(t, got, "nil returned when all tasks are completed")
}

func TestSelectNext_ReturnsNilWhenAllBlocked(t *testing.T) {
	t.Parallel()

	// T-001 depends on T-099 (not in any spec, not completed).
	// T-002 depends on T-098 (same).
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", []string{"T-099"}),
		makeSpec("T-002", []string{"T-098"}),
	}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	assert.Nil(t, got, "nil returned when all tasks are blocked")
}

func TestSelectNext_PhaseNotFound(t *testing.T) {
	t.Parallel()

	sel := NewTaskSelector(nil, emptyStateManager(t), selectorPhases())
	_, err := sel.SelectNext(99)
	assert.Error(t, err)
}

func TestSelectNext_DepsCompletedAllowsTask(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", []string{"T-001"}),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-002", got.ID)
}

func TestSelectNext_SkippedDepDoesNotSatisfy(t *testing.T) {
	t.Parallel()

	// T-002 depends on T-001 which is skipped -- skipped does NOT satisfy a dep.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", []string{"T-001"}),
	}
	sm := writeStateContent(t, []string{
		"T-001|skipped|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	// T-001 is skipped (not not_started), T-002 is not_started but blocked.
	// No actionable task.
	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSelectNext_InProgressNotSelected(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|in_progress|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	// T-001 is in_progress (skip), T-002 is not_started and has no deps.
	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-002", got.ID)
}

func TestSelectNext_MissingStateEntryTreatedAsNotStarted(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
	}
	// State file is empty -- T-001 has no entry.
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-001", got.ID)
}

func TestSelectNext_TasksOutsideSpecSkipped(t *testing.T) {
	t.Parallel()

	// Phase 1 covers T-001 to T-005, but only T-003 has a spec.
	specs := []*ParsedTaskSpec{
		makeSpec("T-003", nil),
	}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	// T-001, T-002 have no spec so they are skipped; T-003 is selected.
	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-003", got.ID)
}

// ---- SelectNextInRange ------------------------------------------------------

func TestSelectNextInRange_SelectsFirstReady(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
		makeSpec("T-003", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|completed|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNextInRange("T-001", "T-005")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-003", got.ID)
}

func TestSelectNextInRange_NilWhenNoneReady(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNextInRange("T-001", "T-003")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSelectNextInRange_InvalidStartTask(t *testing.T) {
	t.Parallel()

	sel := NewTaskSelector(nil, emptyStateManager(t), selectorPhases())
	_, err := sel.SelectNextInRange("invalid", "T-005")
	assert.Error(t, err)
}

func TestSelectNextInRange_InvalidEndTask(t *testing.T) {
	t.Parallel()

	sel := NewTaskSelector(nil, emptyStateManager(t), selectorPhases())
	_, err := sel.SelectNextInRange("T-001", "invalid")
	assert.Error(t, err)
}

func TestSelectNextInRange_StartAfterEnd(t *testing.T) {
	t.Parallel()

	sel := NewTaskSelector(nil, emptyStateManager(t), selectorPhases())
	_, err := sel.SelectNextInRange("T-005", "T-001")
	assert.Error(t, err)
}

func TestSelectNextInRange_SingleTaskRange(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{makeSpec("T-003", nil)}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNextInRange("T-003", "T-003")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-003", got.ID)
}

func TestSelectNextInRange_SpansMultiplePhases(t *testing.T) {
	t.Parallel()

	// T-001 to T-010 spans phase 1 and phase 2.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-006", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNextInRange("T-001", "T-010")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-006", got.ID)
}

// ---- SelectByID -------------------------------------------------------------

func TestSelectByID_ExistingTask(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{makeSpec("T-007", nil)}
	sel := NewTaskSelector(specs, emptyStateManager(t), selectorPhases())

	got, err := sel.SelectByID("T-007")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-007", got.ID)
}

func TestSelectByID_NonExistentTask(t *testing.T) {
	t.Parallel()

	sel := NewTaskSelector(nil, emptyStateManager(t), selectorPhases())
	_, err := sel.SelectByID("T-999")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "T-999")
}

func TestSelectByID_ErrorContainsTaskID(t *testing.T) {
	t.Parallel()

	sel := NewTaskSelector(nil, emptyStateManager(t), selectorPhases())
	_, err := sel.SelectByID("T-042")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "T-042", "error message should include the requested task ID")
}

// ---- GetPhaseProgress -------------------------------------------------------

func TestGetPhaseProgress_CountsCorrectly(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
		makeSpec("T-003", nil),
		makeSpec("T-004", nil),
		makeSpec("T-005", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|in_progress|||",
		"T-003|skipped|||",
		"T-004|blocked|||",
		// T-005 has no entry -> not_started
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	prog, err := sel.GetPhaseProgress(1)
	require.NoError(t, err)

	assert.Equal(t, 1, prog.PhaseID)
	assert.Equal(t, 5, prog.Total)
	assert.Equal(t, 1, prog.Completed)
	assert.Equal(t, 1, prog.InProgress)
	assert.Equal(t, 1, prog.Skipped)
	assert.Equal(t, 1, prog.Blocked)
	assert.Equal(t, 1, prog.NotStarted)
}

func TestGetPhaseProgress_PhaseNotFound(t *testing.T) {
	t.Parallel()

	sel := NewTaskSelector(nil, emptyStateManager(t), selectorPhases())
	_, err := sel.GetPhaseProgress(99)
	assert.Error(t, err)
}

func TestGetPhaseProgress_AllNotStarted(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	prog, err := sel.GetPhaseProgress(1)
	require.NoError(t, err)

	// Phase 1 has T-001 to T-005 (5 total); 2 have specs, 3 have no spec.
	assert.Equal(t, 5, prog.Total)
	assert.Equal(t, 5, prog.NotStarted, "tasks with no spec or no state entry counted as not_started")
	assert.Equal(t, 0, prog.Completed)
}

func TestGetPhaseProgress_EmptyPhase(t *testing.T) {
	t.Parallel()

	phases := []Phase{
		{ID: 5, Name: "Empty Phase", StartTask: "T-050", EndTask: "T-050"},
	}
	sel := NewTaskSelector(nil, emptyStateManager(t), phases)

	prog, err := sel.GetPhaseProgress(5)
	require.NoError(t, err)
	assert.Equal(t, 1, prog.Total) // T-050 is in range but no spec -- counted as not_started
	assert.Equal(t, 0, prog.Completed)
}

// ---- GetAllProgress ---------------------------------------------------------

func TestGetAllProgress_ReturnsBothPhases(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-006", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-006|in_progress|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	all, err := sel.GetAllProgress()
	require.NoError(t, err)

	assert.Len(t, all, 2)
	assert.Contains(t, all, 1)
	assert.Contains(t, all, 2)
	assert.Equal(t, 1, all[1].Completed)
	assert.Equal(t, 1, all[2].InProgress)
}

func TestGetAllProgress_EmptyPhases(t *testing.T) {
	t.Parallel()

	sel := NewTaskSelector(nil, emptyStateManager(t), []Phase{})

	all, err := sel.GetAllProgress()
	require.NoError(t, err)
	assert.Empty(t, all)
}

// ---- IsPhaseComplete --------------------------------------------------------

func TestIsPhaseComplete_AllCompleted(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|completed|||",
	})
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-002"}}
	sel := NewTaskSelector(specs, sm, phases)

	done, err := sel.IsPhaseComplete(1)
	require.NoError(t, err)
	assert.True(t, done)
}

func TestIsPhaseComplete_MixCompletedAndSkipped(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|skipped|||",
	})
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-002"}}
	sel := NewTaskSelector(specs, sm, phases)

	done, err := sel.IsPhaseComplete(1)
	require.NoError(t, err)
	assert.True(t, done, "skipped tasks count as done for phase completion")
}

func TestIsPhaseComplete_InProgressIsNotComplete(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|in_progress|||",
	})
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-002"}}
	sel := NewTaskSelector(specs, sm, phases)

	done, err := sel.IsPhaseComplete(1)
	require.NoError(t, err)
	assert.False(t, done, "in_progress does not count as complete")
}

func TestIsPhaseComplete_NotStartedIsNotComplete(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{makeSpec("T-001", nil)}
	sm := emptyStateManager(t)
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-001"}}
	sel := NewTaskSelector(specs, sm, phases)

	done, err := sel.IsPhaseComplete(1)
	require.NoError(t, err)
	assert.False(t, done)
}

func TestIsPhaseComplete_PhaseNotFound(t *testing.T) {
	t.Parallel()

	sel := NewTaskSelector(nil, emptyStateManager(t), selectorPhases())
	_, err := sel.IsPhaseComplete(99)
	assert.Error(t, err)
}

// ---- BlockedTasks -----------------------------------------------------------

func TestBlockedTasks_ReturnsBlockedSpecs(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", []string{"T-099"}), // T-099 not completed
		makeSpec("T-003", []string{"T-001"}), // T-001 not completed either
	}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	blocked, err := sel.BlockedTasks(1)
	require.NoError(t, err)

	// T-001 is ready (no deps). T-002 and T-003 are blocked.
	ids := make([]string, len(blocked))
	for i, b := range blocked {
		ids[i] = b.ID
	}
	assert.Contains(t, ids, "T-002")
	assert.Contains(t, ids, "T-003")
	assert.NotContains(t, ids, "T-001")
}

func TestBlockedTasks_EmptyWhenAllReady(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	blocked, err := sel.BlockedTasks(1)
	require.NoError(t, err)
	assert.Empty(t, blocked)
}

func TestBlockedTasks_CompletedTasksNotIncluded(t *testing.T) {
	t.Parallel()

	// T-001 is completed and would technically be "blocked" by missing T-099,
	// but since it's completed (not not_started) it should not appear in blocked.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", []string{"T-099"}),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	blocked, err := sel.BlockedTasks(1)
	require.NoError(t, err)
	assert.Empty(t, blocked)
}

func TestBlockedTasks_PhaseNotFound(t *testing.T) {
	t.Parallel()

	sel := NewTaskSelector(nil, emptyStateManager(t), selectorPhases())
	_, err := sel.BlockedTasks(99)
	assert.Error(t, err)
}

// ---- CompletedTaskIDs -------------------------------------------------------

func TestCompletedTaskIDs_ReturnsSorted(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-003", nil),
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := writeStateContent(t, []string{
		"T-003|completed|||",
		"T-001|completed|||",
		"T-002|not_started|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	ids, err := sel.CompletedTaskIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"T-001", "T-003"}, ids)
}

func TestCompletedTaskIDs_EmptyWhenNoneCompleted(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{makeSpec("T-001", nil)}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	ids, err := sel.CompletedTaskIDs()
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestCompletedTaskIDs_SkippedNotIncluded(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|skipped|||",
		"T-002|completed|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	ids, err := sel.CompletedTaskIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"T-002"}, ids, "skipped tasks must not appear in completed list")
}

func TestCompletedTaskIDs_AllCompleted(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
		makeSpec("T-003", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|completed|||",
		"T-003|completed|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	ids, err := sel.CompletedTaskIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"T-001", "T-002", "T-003"}, ids)
}

// ---- RemainingTaskIDs -------------------------------------------------------

func TestRemainingTaskIDs_ExcludesCompletedAndSkipped(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
		makeSpec("T-003", nil),
		makeSpec("T-004", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|skipped|||",
		"T-003|in_progress|||",
		// T-004 no entry -> not_started
	})
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-004"}}
	sel := NewTaskSelector(specs, sm, phases)

	ids, err := sel.RemainingTaskIDs(1)
	require.NoError(t, err)

	assert.NotContains(t, ids, "T-001", "completed excluded")
	assert.NotContains(t, ids, "T-002", "skipped excluded")
	assert.Contains(t, ids, "T-003", "in_progress remains")
	assert.Contains(t, ids, "T-004", "not_started remains")
}

func TestRemainingTaskIDs_ReturnsSorted(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-004", nil),
		makeSpec("T-002", nil),
		makeSpec("T-003", nil),
	}
	sm := emptyStateManager(t)
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-002", EndTask: "T-004"}}
	sel := NewTaskSelector(specs, sm, phases)

	ids, err := sel.RemainingTaskIDs(1)
	require.NoError(t, err)
	assert.Equal(t, []string{"T-002", "T-003", "T-004"}, ids)
}

func TestRemainingTaskIDs_EmptyWhenAllDone(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|skipped|||",
	})
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-002"}}
	sel := NewTaskSelector(specs, sm, phases)

	ids, err := sel.RemainingTaskIDs(1)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestRemainingTaskIDs_PhaseNotFound(t *testing.T) {
	t.Parallel()

	sel := NewTaskSelector(nil, emptyStateManager(t), selectorPhases())
	_, err := sel.RemainingTaskIDs(99)
	assert.Error(t, err)
}

// ---- areDependenciesMet (internal) ------------------------------------------

func TestAreDependenciesMet_NoDeps(t *testing.T) {
	t.Parallel()

	spec := makeSpec("T-001", nil)
	sm := emptyStateManager(t)
	sel := NewTaskSelector([]*ParsedTaskSpec{spec}, sm, selectorPhases())

	met, err := sel.areDependenciesMet(spec)
	require.NoError(t, err)
	assert.True(t, met)
}

func TestAreDependenciesMet_AllCompleted(t *testing.T) {
	t.Parallel()

	spec := makeSpec("T-003", []string{"T-001", "T-002"})
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|completed|||",
	})
	sel := NewTaskSelector([]*ParsedTaskSpec{spec}, sm, selectorPhases())

	met, err := sel.areDependenciesMet(spec)
	require.NoError(t, err)
	assert.True(t, met)
}

func TestAreDependenciesMet_OneMissing(t *testing.T) {
	t.Parallel()

	spec := makeSpec("T-003", []string{"T-001", "T-002"})
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		// T-002 not in state
	})
	sel := NewTaskSelector([]*ParsedTaskSpec{spec}, sm, selectorPhases())

	met, err := sel.areDependenciesMet(spec)
	require.NoError(t, err)
	assert.False(t, met)
}

func TestAreDependenciesMet_SkippedNotSatisfied(t *testing.T) {
	t.Parallel()

	spec := makeSpec("T-002", []string{"T-001"})
	sm := writeStateContent(t, []string{
		"T-001|skipped|||",
	})
	sel := NewTaskSelector([]*ParsedTaskSpec{spec}, sm, selectorPhases())

	met, err := sel.areDependenciesMet(spec)
	require.NoError(t, err)
	assert.False(t, met, "skipped does not satisfy a dependency")
}

func TestAreDependenciesMet_InProgressNotSatisfied(t *testing.T) {
	t.Parallel()

	spec := makeSpec("T-002", []string{"T-001"})
	sm := writeStateContent(t, []string{
		"T-001|in_progress|||",
	})
	sel := NewTaskSelector([]*ParsedTaskSpec{spec}, sm, selectorPhases())

	met, err := sel.areDependenciesMet(spec)
	require.NoError(t, err)
	assert.False(t, met, "in_progress does not satisfy a dependency")
}

// ---- Table-driven integration tests -----------------------------------------

func TestSelectNext_TableDriven(t *testing.T) {
	t.Parallel()

	phases := []Phase{
		{ID: 1, Name: "Phase One", StartTask: "T-001", EndTask: "T-005"},
	}

	tests := []struct {
		name       string
		specs      []*ParsedTaskSpec
		stateLines []string
		wantID     string // empty = expect nil
		wantErr    bool
	}{
		{
			name:    "empty phase returns nil",
			specs:   []*ParsedTaskSpec{},
			wantID:  "",
			wantErr: false,
		},
		{
			name:       "selects first task with no deps",
			specs:      []*ParsedTaskSpec{makeSpec("T-001", nil), makeSpec("T-002", nil)},
			stateLines: []string{},
			wantID:     "T-001",
		},
		{
			name:       "skips completed, selects next",
			specs:      []*ParsedTaskSpec{makeSpec("T-001", nil), makeSpec("T-002", nil)},
			stateLines: []string{"T-001|completed|||"},
			wantID:     "T-002",
		},
		{
			name:       "skips blocked task, selects later ready task",
			specs:      []*ParsedTaskSpec{makeSpec("T-001", []string{"T-099"}), makeSpec("T-002", nil)},
			stateLines: []string{},
			wantID:     "T-002",
		},
		{
			name:       "all blocked returns nil",
			specs:      []*ParsedTaskSpec{makeSpec("T-001", []string{"T-099"}), makeSpec("T-002", []string{"T-098"})},
			stateLines: []string{},
			wantID:     "",
		},
		{
			name:       "all completed returns nil",
			specs:      []*ParsedTaskSpec{makeSpec("T-001", nil)},
			stateLines: []string{"T-001|completed|||"},
			wantID:     "",
		},
		{
			name:       "chain: T-002 ready after T-001 completed",
			specs:      []*ParsedTaskSpec{makeSpec("T-001", nil), makeSpec("T-002", []string{"T-001"})},
			stateLines: []string{"T-001|completed|||"},
			wantID:     "T-002",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sm *StateManager
			if len(tt.stateLines) > 0 {
				sm = writeStateContent(t, tt.stateLines)
			} else {
				sm = emptyStateManager(t)
			}

			sel := NewTaskSelector(tt.specs, sm, phases)
			got, err := sel.SelectNext(1)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.wantID == "" {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.wantID, got.ID)
			}
		})
	}
}

// ---- Self-dependency edge case ----------------------------------------------

func TestSelectNext_SelfDependency(t *testing.T) {
	t.Parallel()

	// A task that lists itself as a dependency can never be completed, so it
	// can never be selected. The selector should skip it and return nil (or
	// pick a later task that has no self-dep).
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", []string{"T-001"}), // self-dependency
	}
	sm := emptyStateManager(t)
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-001"}}
	sel := NewTaskSelector(specs, sm, phases)

	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	assert.Nil(t, got, "self-dependent task must never be selected")
}

func TestSelectNext_SelfDependencyWithFollower(t *testing.T) {
	t.Parallel()

	// T-001 has a self-dep (blocked forever); T-002 has no deps and should
	// still be selected.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", []string{"T-001"}),
		makeSpec("T-002", nil),
	}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-002", got.ID, "T-002 with no deps should be selected despite T-001 being self-blocked")
}

// ---- Circular dependency chain edge case ------------------------------------

func TestSelectNext_CircularChain_AtoB_BtoA(t *testing.T) {
	t.Parallel()

	// T-001 depends on T-002, T-002 depends on T-001. Neither can ever start.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", []string{"T-002"}),
		makeSpec("T-002", []string{"T-001"}),
	}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	assert.Nil(t, got, "circular chain must return nil, not block indefinitely")
}

func TestBlockedTasks_CircularChain(t *testing.T) {
	t.Parallel()

	// A↔B circular chain: both T-001 and T-002 should appear as blocked.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", []string{"T-002"}),
		makeSpec("T-002", []string{"T-001"}),
	}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	blocked, err := sel.BlockedTasks(1)
	require.NoError(t, err)

	ids := make([]string, len(blocked))
	for i, b := range blocked {
		ids[i] = b.ID
	}
	assert.Contains(t, ids, "T-001")
	assert.Contains(t, ids, "T-002")
}

// ---- Cross-phase dependency --------------------------------------------------

func TestSelectNext_CrossPhaseDependency_DepNotCompleted(t *testing.T) {
	t.Parallel()

	// T-006 (phase 2) depends on T-001 (phase 1). T-001 is not yet completed,
	// so T-006 must be blocked.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-006", []string{"T-001"}),
	}
	sm := emptyStateManager(t) // T-001 is not_started
	sel := NewTaskSelector(specs, sm, selectorPhases())

	// Phase 2 only. T-006 depends on T-001 which is not_started.
	got, err := sel.SelectNext(2)
	require.NoError(t, err)
	assert.Nil(t, got, "cross-phase dep not met must block T-006")
}

func TestSelectNext_CrossPhaseDependency_DepCompleted(t *testing.T) {
	t.Parallel()

	// T-006 (phase 2) depends on T-001 (phase 1). T-001 is completed.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-006", []string{"T-001"}),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNext(2)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-006", got.ID, "cross-phase dep satisfied -> T-006 selectable")
}

// ---- SelectNext: in_progress dep does not satisfy blocked task --------------

func TestSelectNext_InProgressDepBlocksTask(t *testing.T) {
	t.Parallel()

	// T-001 is in_progress, T-002 depends on T-001.
	// T-001 is busy (not selectable as not_started), T-002 is blocked.
	// No task is actionable.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", []string{"T-001"}),
	}
	sm := writeStateContent(t, []string{
		"T-001|in_progress|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	assert.Nil(t, got, "in_progress dep must not satisfy T-002; no actionable task")
}

// ---- IsPhaseComplete: all skipped -------------------------------------------

func TestIsPhaseComplete_AllSkipped(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
		makeSpec("T-003", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|skipped|||",
		"T-002|skipped|||",
		"T-003|skipped|||",
	})
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-003"}}
	sel := NewTaskSelector(specs, sm, phases)

	done, err := sel.IsPhaseComplete(1)
	require.NoError(t, err)
	assert.True(t, done, "all-skipped phase must count as complete")
}

func TestIsPhaseComplete_BlockedIsNotComplete(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|blocked|||",
	})
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-002"}}
	sel := NewTaskSelector(specs, sm, phases)

	done, err := sel.IsPhaseComplete(1)
	require.NoError(t, err)
	assert.False(t, done, "blocked status does not count as done for phase completion")
}

// ---- GetPhaseProgress: unmanaged task IDs counted as not_started ------------

func TestGetPhaseProgress_UnmanagedTaskIDsCountedAsNotStarted(t *testing.T) {
	t.Parallel()

	// Phase covers T-001 to T-003, but only T-001 has a spec.
	// T-002 and T-003 have no spec, so they should be counted as not_started.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
	})
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-003"}}
	sel := NewTaskSelector(specs, sm, phases)

	prog, err := sel.GetPhaseProgress(1)
	require.NoError(t, err)

	assert.Equal(t, 3, prog.Total)
	assert.Equal(t, 1, prog.Completed, "T-001 has spec and is completed")
	assert.Equal(t, 2, prog.NotStarted, "T-002 and T-003 have no spec -> not_started")
}

func TestGetPhaseProgress_PhaseIDPreserved(t *testing.T) {
	t.Parallel()

	phases := []Phase{{ID: 7, Name: "Phase Seven", StartTask: "T-001", EndTask: "T-001"}}
	specs := []*ParsedTaskSpec{makeSpec("T-001", nil)}
	sel := NewTaskSelector(specs, emptyStateManager(t), phases)

	prog, err := sel.GetPhaseProgress(7)
	require.NoError(t, err)
	assert.Equal(t, 7, prog.PhaseID)
}

// ---- GetAllProgress: multi-phase aggregation --------------------------------

func TestGetAllProgress_AllPhasesPresent(t *testing.T) {
	t.Parallel()

	phases := []Phase{
		{ID: 1, Name: "Phase One", StartTask: "T-001", EndTask: "T-003"},
		{ID: 2, Name: "Phase Two", StartTask: "T-004", EndTask: "T-006"},
		{ID: 3, Name: "Phase Three", StartTask: "T-007", EndTask: "T-009"},
	}
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-004", nil),
		makeSpec("T-007", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-004|in_progress|||",
		"T-007|not_started|||",
	})
	sel := NewTaskSelector(specs, sm, phases)

	all, err := sel.GetAllProgress()
	require.NoError(t, err)
	require.Len(t, all, 3)

	assert.Equal(t, 1, all[1].Completed)
	assert.Equal(t, 1, all[2].InProgress)
	// T-007 is not_started; T-008 and T-009 have no spec (not_started too).
	assert.Equal(t, 3, all[3].NotStarted)
}

func TestGetAllProgress_SinglePhase(t *testing.T) {
	t.Parallel()

	phases := []Phase{{ID: 1, Name: "Solo", StartTask: "T-001", EndTask: "T-002"}}
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|completed|||",
	})
	sel := NewTaskSelector(specs, sm, phases)

	all, err := sel.GetAllProgress()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, 2, all[1].Completed)
	assert.Equal(t, 0, all[1].NotStarted)
}

// ---- RemainingTaskIDs: includes blocked + in_progress -----------------------

func TestRemainingTaskIDs_IncludesBlockedStatus(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
		makeSpec("T-003", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|blocked|||",
		// T-003 is not_started (no entry)
	})
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-003"}}
	sel := NewTaskSelector(specs, sm, phases)

	ids, err := sel.RemainingTaskIDs(1)
	require.NoError(t, err)

	assert.NotContains(t, ids, "T-001", "completed not remaining")
	assert.Contains(t, ids, "T-002", "blocked is remaining")
	assert.Contains(t, ids, "T-003", "not_started is remaining")
}

func TestRemainingTaskIDs_UnmanagedTaskIDsExcluded(t *testing.T) {
	t.Parallel()

	// Phase covers T-001 to T-003 but only T-001 has a spec. Unmanaged IDs
	// should not appear in RemainingTaskIDs even though they are "not_started".
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
	}
	sm := emptyStateManager(t)
	phases := []Phase{{ID: 1, Name: "P1", StartTask: "T-001", EndTask: "T-003"}}
	sel := NewTaskSelector(specs, sm, phases)

	ids, err := sel.RemainingTaskIDs(1)
	require.NoError(t, err)

	// Only T-001 is managed, so only T-001 should appear.
	assert.Equal(t, []string{"T-001"}, ids)
}

// ---- CompletedTaskIDs: order stability with many specs ---------------------

func TestCompletedTaskIDs_LargeSetSorted(t *testing.T) {
	t.Parallel()

	const n = 20
	specs := make([]*ParsedTaskSpec, n)
	var stateLines []string
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T-%03d", n-i) // specs in reverse order
		specs[i] = makeSpec(id, nil)
		stateLines = append(stateLines, fmt.Sprintf("%s|completed|||", id))
	}
	sm := writeStateContent(t, stateLines)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	ids, err := sel.CompletedTaskIDs()
	require.NoError(t, err)

	assert.Len(t, ids, n)
	for i := 1; i < len(ids); i++ {
		assert.Less(t, ids[i-1], ids[i], "CompletedTaskIDs must be in ascending order")
	}
}

// ---- SelectNextInRange: blocked deps within range ---------------------------

func TestSelectNextInRange_SkipsBlockedRespectsDeps(t *testing.T) {
	t.Parallel()

	// T-002 depends on T-001 (not completed). T-003 has no deps -> selected.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", []string{"T-001"}),
		makeSpec("T-003", nil),
	}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNextInRange("T-001", "T-003")
	require.NoError(t, err)
	require.NotNil(t, got)
	// T-001 has no deps -> should be selected first (not T-003).
	assert.Equal(t, "T-001", got.ID)
}

func TestSelectNextInRange_EmptyRange_NoSpecs(t *testing.T) {
	t.Parallel()

	// Range that has no specs at all.
	sel := NewTaskSelector(nil, emptyStateManager(t), selectorPhases())

	got, err := sel.SelectNextInRange("T-001", "T-005")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSelectNextInRange_AllCompleted(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|completed|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectNextInRange("T-001", "T-002")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// ---- SelectByID: returns exact spec -----------------------------------------

func TestSelectByID_ReturnsCorrectSpec(t *testing.T) {
	t.Parallel()

	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-007", []string{"T-001"}),
		makeSpec("T-010", nil),
	}
	sel := NewTaskSelector(specs, emptyStateManager(t), selectorPhases())

	got, err := sel.SelectByID("T-007")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-007", got.ID)
	// Verify it is the same pointer / correct deps.
	assert.Equal(t, []string{"T-001"}, got.Dependencies)
}

func TestSelectByID_DoesNotConsiderStatus(t *testing.T) {
	t.Parallel()

	// SelectByID must return the spec regardless of the task's current status.
	specs := []*ParsedTaskSpec{makeSpec("T-003", nil)}
	sm := writeStateContent(t, []string{
		"T-003|completed|||",
	})
	sel := NewTaskSelector(specs, sm, selectorPhases())

	got, err := sel.SelectByID("T-003")
	require.NoError(t, err)
	require.NotNil(t, got, "SelectByID must return spec even if task is completed")
	assert.Equal(t, "T-003", got.ID)
}

// ---- Integration test: full phase completion via repeated SelectNext --------

func TestIntegration_FullPhaseCompletion(t *testing.T) {
	t.Parallel()

	// Build a 5-task phase where tasks form a linear chain:
	// T-001 -> T-002 -> T-003 -> T-004 -> T-005
	// Each task depends on the previous one being completed.
	phases := []Phase{{ID: 1, Name: "Integration", StartTask: "T-001", EndTask: "T-005"}}
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", []string{"T-001"}),
		makeSpec("T-003", []string{"T-002"}),
		makeSpec("T-004", []string{"T-003"}),
		makeSpec("T-005", []string{"T-004"}),
	}

	dir := t.TempDir()
	stateFile := fmt.Sprintf("%s/task-state.conf", dir)
	sm := NewStateManager(stateFile)
	require.NoError(t, sm.Initialize([]string{"T-001", "T-002", "T-003", "T-004", "T-005"}))

	sel := NewTaskSelector(specs, sm, phases)

	// Drive the loop: select next, mark completed, repeat.
	expectedOrder := []string{"T-001", "T-002", "T-003", "T-004", "T-005"}
	for _, expectedID := range expectedOrder {
		got, err := sel.SelectNext(1)
		require.NoError(t, err)
		require.NotNil(t, got, "expected task %s to be selectable", expectedID)
		assert.Equal(t, expectedID, got.ID)

		require.NoError(t, sm.UpdateStatus(got.ID, StatusCompleted, "test-agent"))
	}

	// After all tasks are completed, SelectNext must return nil.
	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	assert.Nil(t, got, "phase should be complete after all tasks done")

	// Verify IsPhaseComplete now returns true.
	complete, err := sel.IsPhaseComplete(1)
	require.NoError(t, err)
	assert.True(t, complete)
}

func TestIntegration_DiscoverTasks_FullSelector(t *testing.T) {
	t.Parallel()

	// Load real task specs from the testdata directory and verify the selector
	// works end-to-end.
	specs, err := DiscoverTasks("testdata/task-specs")
	require.NoError(t, err)
	require.Len(t, specs, 3, "testdata/task-specs has T-001, T-002, T-003")

	// Phases cover T-001 to T-003.
	phases := []Phase{{ID: 1, Name: "Test Phase", StartTask: "T-001", EndTask: "T-003"}}

	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, phases)

	// T-001 and T-002 have no deps, T-003 depends on T-001 and T-002.
	// First selection must be T-001.
	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-001", got.ID)

	// Mark T-001 completed.
	require.NoError(t, sm.UpdateStatus("T-001", StatusCompleted, "agent"))

	// Next should be T-002 (no deps).
	got, err = sel.SelectNext(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-002", got.ID)

	// Mark T-002 completed.
	require.NoError(t, sm.UpdateStatus("T-002", StatusCompleted, "agent"))

	// Now T-003's deps (T-001, T-002) are both completed -> T-003 selectable.
	got, err = sel.SelectNext(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-003", got.ID)

	// Mark T-003 completed.
	require.NoError(t, sm.UpdateStatus("T-003", StatusCompleted, "agent"))

	// Phase should now be complete.
	got, err = sel.SelectNext(1)
	require.NoError(t, err)
	assert.Nil(t, got, "all tasks done -> nil")

	complete, err := sel.IsPhaseComplete(1)
	require.NoError(t, err)
	assert.True(t, complete)
}

func TestIntegration_MixedPhaseProgress(t *testing.T) {
	t.Parallel()

	// Simulate a phase with 5 tasks:
	// T-001: completed
	// T-002: skipped
	// T-003: in_progress
	// T-004: not_started, no deps -> selectable
	// T-005: blocked (dep on T-003 which is in_progress)
	phases := []Phase{{ID: 1, Name: "Mixed", StartTask: "T-001", EndTask: "T-005"}}
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
		makeSpec("T-003", nil),
		makeSpec("T-004", nil),
		makeSpec("T-005", []string{"T-003"}),
	}
	sm := writeStateContent(t, []string{
		"T-001|completed|||",
		"T-002|skipped|||",
		"T-003|in_progress|||",
		// T-004 no entry -> not_started
		// T-005 no entry -> not_started, but dep T-003 is in_progress
	})
	sel := NewTaskSelector(specs, sm, phases)

	// SelectNext should return T-004 (T-003 is in_progress, T-005's dep not met).
	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-004", got.ID)

	// Phase is not complete.
	complete, err := sel.IsPhaseComplete(1)
	require.NoError(t, err)
	assert.False(t, complete)

	// Progress check.
	prog, err := sel.GetPhaseProgress(1)
	require.NoError(t, err)
	assert.Equal(t, 5, prog.Total)
	assert.Equal(t, 1, prog.Completed)
	assert.Equal(t, 1, prog.Skipped)
	assert.Equal(t, 1, prog.InProgress)
	assert.Equal(t, 2, prog.NotStarted)

	// Remaining: T-003 (in_progress), T-004 (not_started), T-005 (not_started).
	remaining, err := sel.RemainingTaskIDs(1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"T-003", "T-004", "T-005"}, remaining)

	// Blocked: only T-005 is not_started with dep T-003 not completed.
	blocked, err := sel.BlockedTasks(1)
	require.NoError(t, err)
	require.Len(t, blocked, 1)
	assert.Equal(t, "T-005", blocked[0].ID)
}

// ---- Table-driven tests for areDependenciesMetFromMap -----------------------

func TestAreDependenciesMetFromMap_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		specDeps   []string
		stateLines []string
		wantMet    bool
	}{
		{
			name:     "no dependencies always met",
			specDeps: nil,
			wantMet:  true,
		},
		{
			name:       "single dep completed",
			specDeps:   []string{"T-001"},
			stateLines: []string{"T-001|completed|||"},
			wantMet:    true,
		},
		{
			name:       "single dep not_started",
			specDeps:   []string{"T-001"},
			stateLines: []string{"T-001|not_started|||"},
			wantMet:    false,
		},
		{
			name:       "single dep in_progress",
			specDeps:   []string{"T-001"},
			stateLines: []string{"T-001|in_progress|||"},
			wantMet:    false,
		},
		{
			name:       "single dep skipped",
			specDeps:   []string{"T-001"},
			stateLines: []string{"T-001|skipped|||"},
			wantMet:    false,
		},
		{
			name:       "single dep blocked",
			specDeps:   []string{"T-001"},
			stateLines: []string{"T-001|blocked|||"},
			wantMet:    false,
		},
		{
			name:       "all deps completed",
			specDeps:   []string{"T-001", "T-002", "T-003"},
			stateLines: []string{"T-001|completed|||", "T-002|completed|||", "T-003|completed|||"},
			wantMet:    true,
		},
		{
			name:       "one dep missing from state (treated as not_started)",
			specDeps:   []string{"T-001", "T-002"},
			stateLines: []string{"T-001|completed|||"},
			wantMet:    false,
		},
		{
			name:       "dep not in state at all",
			specDeps:   []string{"T-099"},
			stateLines: nil,
			wantMet:    false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := makeSpec("T-010", tt.specDeps)
			var sm *StateManager
			if len(tt.stateLines) > 0 {
				sm = writeStateContent(t, tt.stateLines)
			} else {
				sm = emptyStateManager(t)
			}
			sel := NewTaskSelector([]*ParsedTaskSpec{spec}, sm, selectorPhases())

			met, err := sel.areDependenciesMet(spec)
			require.NoError(t, err)
			assert.Equal(t, tt.wantMet, met)
		})
	}
}

// ---- Duplicate task IDs in spec list ----------------------------------------

func TestNewTaskSelector_DuplicateSpecIDs_LastWins(t *testing.T) {
	t.Parallel()

	// If the caller provides duplicate IDs, the last spec for that ID wins
	// in the specMap (constructor does not reject them; caller must deduplicate).
	firstSpec := &ParsedTaskSpec{ID: "T-001", Title: "First", Dependencies: []string{}}
	secondSpec := &ParsedTaskSpec{ID: "T-001", Title: "Second", Dependencies: []string{}}

	sel := NewTaskSelector([]*ParsedTaskSpec{firstSpec, secondSpec}, emptyStateManager(t), selectorPhases())
	require.NotNil(t, sel)
	assert.Len(t, sel.specMap, 1, "duplicate IDs collapse into one entry in specMap")
	assert.Equal(t, "Second", sel.specMap["T-001"].Title, "last spec wins for duplicate IDs")
}

// ---- Phase with no tasks (empty range boundary) -----------------------------

func TestSelectNext_EmptyPhaseRange(t *testing.T) {
	t.Parallel()

	// A phase where StartTask > EndTask yields no tasks -> nil, nil.
	phases := []Phase{{ID: 1, Name: "Bad Range", StartTask: "T-010", EndTask: "T-001"}}
	sel := NewTaskSelector(nil, emptyStateManager(t), phases)

	got, err := sel.SelectNext(1)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestIsPhaseComplete_EmptyPhaseRange(t *testing.T) {
	t.Parallel()

	// Phase with StartTask > EndTask produces no task IDs, so it is vacuously
	// complete.
	phases := []Phase{{ID: 1, Name: "Bad Range", StartTask: "T-010", EndTask: "T-001"}}
	sel := NewTaskSelector(nil, emptyStateManager(t), phases)

	done, err := sel.IsPhaseComplete(1)
	require.NoError(t, err)
	assert.True(t, done, "empty range is vacuously complete")
}

func TestBlockedTasks_EmptyPhaseRange(t *testing.T) {
	t.Parallel()

	phases := []Phase{{ID: 1, Name: "Bad Range", StartTask: "T-010", EndTask: "T-001"}}
	sel := NewTaskSelector(nil, emptyStateManager(t), phases)

	blocked, err := sel.BlockedTasks(1)
	require.NoError(t, err)
	assert.Empty(t, blocked)
}

func TestRemainingTaskIDs_EmptyPhaseRange(t *testing.T) {
	t.Parallel()

	phases := []Phase{{ID: 1, Name: "Bad Range", StartTask: "T-010", EndTask: "T-001"}}
	sel := NewTaskSelector(nil, emptyStateManager(t), phases)

	ids, err := sel.RemainingTaskIDs(1)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

// ---- Table-driven: SelectNext respects phase boundaries ---------------------

func TestSelectNext_RespectsPhaseRange(t *testing.T) {
	t.Parallel()

	// T-001 to T-005 belong to phase 1; T-006 to T-010 to phase 2.
	// All tasks are not_started with no deps.
	specs := []*ParsedTaskSpec{
		makeSpec("T-001", nil),
		makeSpec("T-002", nil),
		makeSpec("T-003", nil),
		makeSpec("T-006", nil),
		makeSpec("T-007", nil),
	}
	sm := emptyStateManager(t)
	sel := NewTaskSelector(specs, sm, selectorPhases())

	// Phase 2 should only see T-006 through T-010.
	got, err := sel.SelectNext(2)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T-006", got.ID, "phase 2 selector must not return T-001 from phase 1")
}

// ---- Large spec set benchmarks ----------------------------------------------

func BenchmarkSelectNext_LargePhase(b *testing.B) {
	const n = 200
	specs := make([]*ParsedTaskSpec, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		var deps []string
		if i > 0 {
			deps = []string{fmt.Sprintf("T-%03d", i)}
		}
		specs[i] = makeSpec(id, deps)
	}

	phases := []Phase{{ID: 1, Name: "Large", StartTask: "T-001", EndTask: fmt.Sprintf("T-%03d", n)}}

	// Mark the first half as completed.
	var lines []string
	for i := 1; i <= n/2; i++ {
		lines = append(lines, fmt.Sprintf("T-%03d|completed|||", i))
	}

	dir := b.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	content := strings.Join(lines, "\n") + "\n"
	_ = os.WriteFile(path, []byte(content), 0644)
	sm := NewStateManager(path)

	sel := NewTaskSelector(specs, sm, phases)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sel.SelectNext(1)
	}
}

func BenchmarkGetPhaseProgress_LargePhase(b *testing.B) {
	const n = 200
	specs := make([]*ParsedTaskSpec, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		specs[i] = makeSpec(id, nil)
	}

	phases := []Phase{{ID: 1, Name: "Large", StartTask: "T-001", EndTask: fmt.Sprintf("T-%03d", n)}}

	var lines []string
	for i := 1; i <= n; i++ {
		status := "not_started"
		if i <= n/4 {
			status = "completed"
		} else if i <= n/2 {
			status = "in_progress"
		}
		lines = append(lines, fmt.Sprintf("T-%03d|%s|||", i, status))
	}

	dir := b.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	content := strings.Join(lines, "\n") + "\n"
	_ = os.WriteFile(path, []byte(content), 0644)
	sm := NewStateManager(path)

	sel := NewTaskSelector(specs, sm, phases)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sel.GetPhaseProgress(1)
	}
}

func BenchmarkGetAllProgress_MultiplePhases(b *testing.B) {
	const tasksPerPhase = 50
	const numPhases = 4

	specs := make([]*ParsedTaskSpec, 0, tasksPerPhase*numPhases)
	phases := make([]Phase, numPhases)
	var lines []string

	for p := 0; p < numPhases; p++ {
		startNum := p*tasksPerPhase + 1
		endNum := (p + 1) * tasksPerPhase
		phases[p] = Phase{
			ID:        p + 1,
			Name:      fmt.Sprintf("Phase %d", p+1),
			StartTask: fmt.Sprintf("T-%03d", startNum),
			EndTask:   fmt.Sprintf("T-%03d", endNum),
		}
		for i := startNum; i <= endNum; i++ {
			id := fmt.Sprintf("T-%03d", i)
			specs = append(specs, makeSpec(id, nil))
			if i <= startNum+10 {
				lines = append(lines, fmt.Sprintf("%s|completed|||", id))
			}
		}
	}

	dir := b.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	content := strings.Join(lines, "\n") + "\n"
	_ = os.WriteFile(path, []byte(content), 0644)
	sm := NewStateManager(path)

	sel := NewTaskSelector(specs, sm, phases)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sel.GetAllProgress()
	}
}

func BenchmarkCompletedTaskIDs_LargeSet(b *testing.B) {
	const n = 200
	specs := make([]*ParsedTaskSpec, n)
	var lines []string
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T-%03d", i+1)
		specs[i] = makeSpec(id, nil)
		if i%2 == 0 {
			lines = append(lines, fmt.Sprintf("%s|completed|||", id))
		}
	}

	dir := b.TempDir()
	path := filepath.Join(dir, "task-state.conf")
	content := strings.Join(lines, "\n") + "\n"
	_ = os.WriteFile(path, []byte(content), 0644)
	sm := NewStateManager(path)

	sel := NewTaskSelector(specs, sm, selectorPhases())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sel.CompletedTaskIDs()
	}
}
