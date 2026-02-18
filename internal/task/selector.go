package task

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/log"
)

// PhaseProgress holds aggregate task counts for a single phase.
type PhaseProgress struct {
	// PhaseID is the numeric identifier of the phase.
	PhaseID int
	// Total is the number of task IDs that fall within the phase range.
	Total int
	// Completed is the count of tasks with StatusCompleted.
	Completed int
	// InProgress is the count of tasks with StatusInProgress.
	InProgress int
	// Blocked is the count of tasks with at least one incomplete dependency.
	Blocked int
	// Skipped is the count of tasks with StatusSkipped.
	Skipped int
	// NotStarted is the count of tasks with StatusNotStarted or no state entry.
	NotStarted int
}

// TaskSelector selects the next actionable task from a set of parsed specs,
// consulting the StateManager for current task status. It is read-only: it
// never modifies state.
type TaskSelector struct {
	specs   []*ParsedTaskSpec
	state   *StateManager
	phases  []Phase
	specMap map[string]*ParsedTaskSpec
}

// NewTaskSelector constructs a TaskSelector. specs is the complete list of
// parsed task specs; state is the StateManager used to query task status;
// phases is the slice of Phase configurations used to enumerate phase task IDs.
//
// A specMap is built from specs for O(1) lookup by task ID.
func NewTaskSelector(specs []*ParsedTaskSpec, state *StateManager, phases []Phase) *TaskSelector {
	specMap := make(map[string]*ParsedTaskSpec, len(specs))
	for _, s := range specs {
		specMap[s.ID] = s
	}
	return &TaskSelector{
		specs:   specs,
		state:   state,
		phases:  phases,
		specMap: specMap,
	}
}

// SelectNext returns the first task in phaseID that is not_started and has all
// dependencies completed. Tasks are evaluated in the order returned by
// TasksInPhase (numeric ascending). Returns nil, nil when no task is currently
// actionable (all done, all blocked, or phase is empty / not found).
func (s *TaskSelector) SelectNext(phaseID int) (*ParsedTaskSpec, error) {
	phase := PhaseByID(s.phases, phaseID)
	if phase == nil {
		return nil, fmt.Errorf("selecting next task: phase %d not found", phaseID)
	}

	// Load state once for the entire scan -- avoids N file reads per phase.
	stateMap, err := s.state.LoadMap()
	if err != nil {
		return nil, fmt.Errorf("selecting next task in phase %d: loading state: %w", phaseID, err)
	}

	ids := TasksInPhase(*phase)
	for _, id := range ids {
		spec, ok := s.specMap[id]
		if !ok {
			// Task ID is in the phase range but has no spec file -- skip it.
			log.Debug("task has no spec, skipping", "task", id, "phase", phaseID)
			continue
		}

		// Missing state entry is treated as not_started.
		status := StatusNotStarted
		if ts, exists := stateMap[id]; exists {
			status = ts.Status
		}

		if status != StatusNotStarted {
			continue
		}

		met := areDependenciesMetFromMap(spec, stateMap)
		if !met {
			log.Debug("task dependencies not met, skipping", "task", id)
			continue
		}

		log.Debug("selected next task", "task", id, "phase", phaseID)
		return spec, nil
	}

	return nil, nil
}

// SelectNextInRange returns the first task whose numeric ID falls within the
// inclusive range [startTask, endTask] that is not_started and has all
// dependencies completed. This is used by --phase all mode spanning multiple
// phases. Returns nil, nil when no actionable task exists in the range.
func (s *TaskSelector) SelectNextInRange(startTask, endTask string) (*ParsedTaskSpec, error) {
	startNum, err := TaskIDNumber(startTask)
	if err != nil {
		return nil, fmt.Errorf("selecting next task in range: invalid start task %q: %w", startTask, err)
	}
	endNum, err := TaskIDNumber(endTask)
	if err != nil {
		return nil, fmt.Errorf("selecting next task in range: invalid end task %q: %w", endTask, err)
	}
	if startNum > endNum {
		return nil, fmt.Errorf("selecting next task in range: start task %q is after end task %q", startTask, endTask)
	}

	// Load state once for the entire scan.
	stateMap, err := s.state.LoadMap()
	if err != nil {
		return nil, fmt.Errorf("selecting next task in range [%s,%s]: loading state: %w", startTask, endTask, err)
	}

	for i := startNum; i <= endNum; i++ {
		id := fmt.Sprintf("T-%03d", i)
		spec, ok := s.specMap[id]
		if !ok {
			continue
		}

		status := StatusNotStarted
		if ts, exists := stateMap[id]; exists {
			status = ts.Status
		}

		if status != StatusNotStarted {
			continue
		}

		if !areDependenciesMetFromMap(spec, stateMap) {
			continue
		}

		log.Debug("selected next task in range", "task", id, "start", startTask, "end", endTask)
		return spec, nil
	}

	return nil, nil
}

// SelectByID returns the ParsedTaskSpec for the given task ID. Returns an
// error if taskID is not present in the spec map.
func (s *TaskSelector) SelectByID(taskID string) (*ParsedTaskSpec, error) {
	spec, ok := s.specMap[taskID]
	if !ok {
		return nil, fmt.Errorf("task %s not found in spec map", taskID)
	}
	return spec, nil
}

// GetPhaseProgress returns aggregate status counts for the given phaseID.
// An error is returned if the phase is not found or state cannot be queried.
func (s *TaskSelector) GetPhaseProgress(phaseID int) (PhaseProgress, error) {
	phase := PhaseByID(s.phases, phaseID)
	if phase == nil {
		return PhaseProgress{}, fmt.Errorf("getting phase progress: phase %d not found", phaseID)
	}

	prog, err := s.phaseProgressFor(*phase)
	if err != nil {
		return PhaseProgress{}, err
	}
	return prog, nil
}

// GetAllProgress returns a map of phase ID to PhaseProgress for every phase in
// the selector's phase list.
func (s *TaskSelector) GetAllProgress() (map[int]PhaseProgress, error) {
	result := make(map[int]PhaseProgress, len(s.phases))
	for _, phase := range s.phases {
		prog, err := s.phaseProgressFor(phase)
		if err != nil {
			return nil, fmt.Errorf("getting all progress: %w", err)
		}
		result[phase.ID] = prog
	}
	return result, nil
}

// IsPhaseComplete returns true if every task in phaseID is either completed or
// skipped. An in_progress or not_started task prevents completion.
func (s *TaskSelector) IsPhaseComplete(phaseID int) (bool, error) {
	phase := PhaseByID(s.phases, phaseID)
	if phase == nil {
		return false, fmt.Errorf("checking phase completion: phase %d not found", phaseID)
	}

	ids := TasksInPhase(*phase)
	stateMap, err := s.state.LoadMap()
	if err != nil {
		return false, fmt.Errorf("checking phase completion for phase %d: %w", phaseID, err)
	}

	for _, id := range ids {
		if _, inSpec := s.specMap[id]; !inSpec {
			// No spec means this task ID is unmanaged -- treat as not-started.
			// A phase cannot be complete if unmanaged task IDs exist.
			continue
		}

		ts, exists := stateMap[id]
		status := StatusNotStarted
		if exists {
			status = ts.Status
		}

		// Only completed and skipped count as "done" for phase completion.
		if status != StatusCompleted && status != StatusSkipped {
			return false, nil
		}
	}

	return true, nil
}

// BlockedTasks returns specs for tasks in phaseID that are not_started but
// have at least one incomplete dependency.
func (s *TaskSelector) BlockedTasks(phaseID int) ([]*ParsedTaskSpec, error) {
	phase := PhaseByID(s.phases, phaseID)
	if phase == nil {
		return nil, fmt.Errorf("listing blocked tasks: phase %d not found", phaseID)
	}

	stateMap, err := s.state.LoadMap()
	if err != nil {
		return nil, fmt.Errorf("listing blocked tasks in phase %d: loading state: %w", phaseID, err)
	}

	ids := TasksInPhase(*phase)
	var blocked []*ParsedTaskSpec

	for _, id := range ids {
		spec, ok := s.specMap[id]
		if !ok {
			continue
		}

		status := StatusNotStarted
		if ts, exists := stateMap[id]; exists {
			status = ts.Status
		}

		if status != StatusNotStarted {
			continue
		}

		if !areDependenciesMetFromMap(spec, stateMap) {
			blocked = append(blocked, spec)
		}
	}

	return blocked, nil
}

// CompletedTaskIDs returns a sorted list of all task IDs (across all specs)
// that have StatusCompleted in the state manager.
func (s *TaskSelector) CompletedTaskIDs() ([]string, error) {
	stateMap, err := s.state.LoadMap()
	if err != nil {
		return nil, fmt.Errorf("listing completed task IDs: %w", err)
	}

	var ids []string
	for _, spec := range s.specs {
		ts, exists := stateMap[spec.ID]
		if exists && ts.Status == StatusCompleted {
			ids = append(ids, spec.ID)
		}
	}

	sort.Strings(ids)
	return ids, nil
}

// RemainingTaskIDs returns a sorted list of task IDs in phaseID that are not
// completed or skipped (i.e., still need work).
func (s *TaskSelector) RemainingTaskIDs(phaseID int) ([]string, error) {
	phase := PhaseByID(s.phases, phaseID)
	if phase == nil {
		return nil, fmt.Errorf("listing remaining tasks: phase %d not found", phaseID)
	}

	ids := TasksInPhase(*phase)
	stateMap, err := s.state.LoadMap()
	if err != nil {
		return nil, fmt.Errorf("listing remaining tasks in phase %d: %w", phaseID, err)
	}

	var remaining []string
	for _, id := range ids {
		if _, inSpec := s.specMap[id]; !inSpec {
			continue
		}

		ts, exists := stateMap[id]
		status := StatusNotStarted
		if exists {
			status = ts.Status
		}

		if status != StatusCompleted && status != StatusSkipped {
			remaining = append(remaining, id)
		}
	}

	sort.Strings(remaining)
	return remaining, nil
}

// areDependenciesMet returns true if every dependency ID in spec.Dependencies
// has StatusCompleted in the state manager. Missing state entries are treated
// as not_started (not completed). Skipped does NOT satisfy a dependency.
func (s *TaskSelector) areDependenciesMet(spec *ParsedTaskSpec) (bool, error) {
	if len(spec.Dependencies) == 0 {
		return true, nil
	}

	stateMap, err := s.state.LoadMap()
	if err != nil {
		return false, fmt.Errorf("checking dependencies for %s: %w", spec.ID, err)
	}

	return areDependenciesMetFromMap(spec, stateMap), nil
}

// areDependenciesMetFromMap checks whether all dependencies in spec are
// completed according to the provided stateMap snapshot. Missing entries are
// treated as not_started. Only StatusCompleted satisfies a dependency.
func areDependenciesMetFromMap(spec *ParsedTaskSpec, stateMap map[string]*TaskState) bool {
	for _, depID := range spec.Dependencies {
		ts, exists := stateMap[depID]
		if !exists {
			return false
		}
		if ts.Status != StatusCompleted {
			return false
		}
	}
	return true
}

// phaseProgressFor computes aggregate counts for a single Phase.
func (s *TaskSelector) phaseProgressFor(phase Phase) (PhaseProgress, error) {
	ids := TasksInPhase(phase)
	prog := PhaseProgress{PhaseID: phase.ID, Total: len(ids)}

	stateMap, err := s.state.LoadMap()
	if err != nil {
		return PhaseProgress{}, fmt.Errorf("computing progress for phase %d: %w", phase.ID, err)
	}

	for _, id := range ids {
		if _, inSpec := s.specMap[id]; !inSpec {
			// Unmanaged task ID: no spec, count as not_started.
			prog.NotStarted++
			continue
		}

		ts, exists := stateMap[id]
		status := StatusNotStarted
		if exists {
			status = ts.Status
		}

		switch status {
		case StatusCompleted:
			prog.Completed++
		case StatusInProgress:
			prog.InProgress++
		case StatusSkipped:
			prog.Skipped++
		case StatusBlocked:
			prog.Blocked++
		default:
			// not_started or any unrecognized status.
			prog.NotStarted++
		}
	}

	return prog, nil
}
