# T-019: Dependency Resolution and Next-Task Selection

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-004, T-016, T-017, T-018 |
| Blocked By | T-016, T-017, T-018 |
| Blocks | T-020, T-027, T-029 |

## Goal
Implement the dependency resolution engine and next-task selector that determines which task should be worked on next. Given a phase range, it finds the first `not_started` task whose dependencies are all `completed`, enabling the implementation loop to autonomously work through tasks in the correct order without human intervention.

## Background
Per PRD Section 5.3, `select_next_task(phase_range)` returns the first `not_started` task whose dependencies are all `completed`. This is described as "a simple topological check (not a full sort) -- check that all dependencies are completed before selecting a task." The selector must cross-reference three data sources: task specs (for dependency declarations from T-016), task state (for current statuses from T-017), and phase config (for scoping to a phase range from T-018). The PRD also mentions "For parallel task execution, build a dependency graph and identify independent sets that can run concurrently" -- this is a v2.1 feature but the data structures should support it.

## Technical Specifications
### Implementation Approach
Create `internal/task/selector.go` containing a `TaskSelector` that combines parsed task specs, task state, and phase configuration to determine the next task to work on. The core algorithm iterates through tasks in a phase (ordered by ID), checks each task's status, and for `not_started` tasks, verifies all dependencies are `completed`. The first such task is selected. Also provide methods for querying task progress and identifying blocked tasks.

### Key Components
- **TaskSelector**: Main selector that combines task specs, state, and phases
- **SelectNext**: Core method that returns the next actionable task
- **PhaseProgress**: Calculates completion statistics for a phase
- **DetectBlocked**: Identifies tasks that cannot proceed due to incomplete dependencies
- **DependencyGraph**: Lightweight graph representation for dependency analysis

### API/Interface Contracts
```go
// internal/task/selector.go
package task

// TaskSelector selects the next task to work on based on dependencies and state.
type TaskSelector struct {
    specs    []*ParsedTaskSpec      // All parsed task specs
    state    *StateManager          // Task state manager
    phases   []Phase                // Phase configuration
    specMap  map[string]*ParsedTaskSpec // Task ID -> spec lookup
}

// NewTaskSelector creates a TaskSelector from the component parts.
func NewTaskSelector(specs []*ParsedTaskSpec, state *StateManager, phases []Phase) *TaskSelector

// SelectNext returns the next actionable task in the given phase.
// A task is actionable if:
//   1. It falls within the phase's task range
//   2. Its status is not_started
//   3. All its dependencies have status completed
// Returns the first such task (by ID order), or nil if no task is actionable.
// Returns (nil, nil) when all tasks in the phase are completed.
func (ts *TaskSelector) SelectNext(phaseID int) (*ParsedTaskSpec, error)

// SelectNextInRange returns the next actionable task within an explicit
// task ID range (start, end inclusive). Used when --phase all spans
// multiple phases.
func (ts *TaskSelector) SelectNextInRange(startTask, endTask string) (*ParsedTaskSpec, error)

// SelectByID returns a specific task by ID, regardless of phase or status.
// Used for --task T-007 mode.
func (ts *TaskSelector) SelectByID(taskID string) (*ParsedTaskSpec, error)

// PhaseProgress returns completion statistics for a phase.
type PhaseProgress struct {
    PhaseID     int
    PhaseName   string
    Total       int
    Completed   int
    InProgress  int
    Blocked     int
    Skipped     int
    NotStarted  int
}

// GetPhaseProgress calculates progress statistics for a specific phase.
func (ts *TaskSelector) GetPhaseProgress(phaseID int) (*PhaseProgress, error)

// GetAllProgress returns progress for all phases.
func (ts *TaskSelector) GetAllProgress() ([]PhaseProgress, error)

// IsPhaseComplete returns true if all tasks in a phase are completed or skipped.
func (ts *TaskSelector) IsPhaseComplete(phaseID int) (bool, error)

// BlockedTasks returns tasks that are not_started but have incomplete dependencies.
func (ts *TaskSelector) BlockedTasks(phaseID int) ([]*ParsedTaskSpec, error)

// CompletedTaskIDs returns the IDs of all completed tasks (across all phases).
// Used for populating {{COMPLETED_TASKS}} in prompt templates.
func (ts *TaskSelector) CompletedTaskIDs() ([]string, error)

// RemainingTaskIDs returns the IDs of all non-completed tasks in a phase.
// Used for populating {{REMAINING_TASKS}} in prompt templates.
func (ts *TaskSelector) RemainingTaskIDs(phaseID int) ([]string, error)

// areDependenciesMet checks if all dependencies of a task are completed.
func (ts *TaskSelector) areDependenciesMet(spec *ParsedTaskSpec) (bool, error)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/task (T-016) | - | ParsedTaskSpec, DiscoverTasks |
| internal/task (T-017) | - | StateManager for status queries |
| internal/task (T-018) | - | Phase configuration, TasksInPhase |
| fmt | stdlib | Error wrapping |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] SelectNext returns the first not_started task with all deps completed
- [ ] SelectNext returns nil when all tasks in phase are completed
- [ ] SelectNext skips tasks whose dependencies are not yet completed
- [ ] SelectNext respects phase boundaries (only considers tasks in the given phase)
- [ ] SelectByID returns the specific task regardless of phase
- [ ] SelectByID returns error for non-existent task ID
- [ ] GetPhaseProgress returns correct counts for each status category
- [ ] GetAllProgress returns progress for all phases
- [ ] IsPhaseComplete returns true when all tasks are completed or skipped
- [ ] BlockedTasks identifies tasks with incomplete dependencies
- [ ] CompletedTaskIDs returns all completed task IDs across phases
- [ ] RemainingTaskIDs returns non-completed task IDs in a phase
- [ ] Cross-phase dependencies are respected (task in phase 2 can depend on task in phase 1)
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- 3 tasks, all not_started, no dependencies: SelectNext returns first task
- 3 tasks: T-001 completed, T-002 depends on T-001, T-003 depends on T-002: SelectNext returns T-002
- 3 tasks: T-001 in_progress, T-002 depends on T-001: SelectNext returns nil (T-002 blocked, T-001 busy)
- All tasks completed: SelectNext returns nil
- Single task with no dependencies: SelectNext returns it
- Task with dependency on task in a different phase: dependency check crosses phases
- Phase 2 with 5 tasks, 3 completed: GetPhaseProgress shows Total=5, Completed=3, NotStarted=2
- Phase with all tasks completed: IsPhaseComplete returns true
- Phase with one skipped, rest completed: IsPhaseComplete returns true
- Phase with one in_progress: IsPhaseComplete returns false
- BlockedTasks returns tasks whose deps are not_started or in_progress
- SelectByID("T-007"): returns the specific task
- SelectByID("T-999"): returns error
- CompletedTaskIDs returns sorted list of completed tasks
- RemainingTaskIDs returns sorted list of non-completed tasks

### Integration Tests
- Full scenario: load task specs from testdata, load state, load phases, run SelectNext through a complete phase
- Verify that repeatedly calling SelectNext + updating state to completed eventually returns nil (phase complete)

### Edge Cases to Handle
- Task with dependency on itself (circular -- should be caught or ignored)
- Task with dependency on non-existent task ID (return error or skip)
- Phase with no tasks (empty range): SelectNext returns nil, IsPhaseComplete returns true
- All tasks blocked (circular dependency chain): SelectNext returns nil with no completed tasks
- Task state file missing (treat all tasks as not_started)
- Duplicate task IDs in different phases (should not happen, but handle gracefully)
- Very large task set (200+ tasks): performance should be O(n) per SelectNext call

## Implementation Notes
### Recommended Approach
1. Constructor builds `specMap` for O(1) task lookup by ID
2. SelectNext: get phase's task range, iterate in order, for each not_started task check deps
3. areDependenciesMet: for each dependency ID, look up status in StateManager, all must be completed
4. PhaseProgress: iterate all tasks in phase, count statuses
5. CompletedTaskIDs/RemainingTaskIDs: iterate all tasks, filter by status
6. Use table-driven tests with a test helper that builds task specs and state in memory
7. For integration tests, create a `testdata/` directory with a small but complete project (5-10 tasks)

### Potential Pitfalls
- Dependencies reference task IDs, not task specs -- a dependency on "T-001" means the task with ID "T-001" must be completed, not that the spec file for T-001 must exist
- Task state file may not have entries for all discovered tasks -- tasks without state entries are implicitly not_started
- When a task's dependency is `skipped`, the dependent task should be considered blocked (skipped != completed)
- Performance: for large task sets, pre-compute a dependency lookup map rather than scanning all deps on every call
- SelectNext should not modify state -- it is a read-only query

### Security Considerations
- No security-sensitive operations in this component
- Input validation: ensure task IDs follow the T-NNN format before lookup

## References
- [PRD Section 5.3 - Task Management System](docs/prd/PRD-Raven.md)
- [PRD Section 5.3 - Dependency resolution](docs/prd/PRD-Raven.md)
- [Topological Sort - Kahn's Algorithm](https://en.wikipedia.org/wiki/Topological_sorting#Kahn's_algorithm)
