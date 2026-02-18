package prd

import (
	"fmt"
	"sort"
)

// IDMapping maps a task's temp_id to its assigned global_id.
// For example: "E001-T01" -> "T-001".
type IDMapping map[string]string

// MergedTask holds a task with its assigned global ID, retaining the original
// temp ID and all fields from the source TaskDef.
type MergedTask struct {
	// GlobalID is the sequential global identifier in T-NNN (or T-NNNN) format.
	GlobalID string
	// TempID is the original temporary task identifier (e.g., E001-T01).
	TempID string
	// EpicID is the source epic identifier in E-NNN format.
	EpicID string
	// Title is the short human-readable name for the task.
	Title string
	// Description explains what the task implements.
	Description string
	// AcceptanceCriteria lists the conditions for task completion.
	AcceptanceCriteria []string
	// LocalDependencies lists temp_ids of tasks within the same epic (not yet resolved).
	LocalDependencies []string
	// CrossEpicDeps lists cross-epic dependency references in "E-NNN:label" format (not yet resolved).
	CrossEpicDeps []string
	// Effort is the size estimate; one of: "small", "medium", "large".
	Effort string
	// Priority is the importance classification; one of: "must-have", "should-have", "nice-to-have".
	Priority string
}

// SortEpicsByDependency returns epic IDs in topological order using Kahn's algorithm.
// Epics with no dependencies are placed first, sorted lexicographically for determinism.
// Returns an error if a cycle is detected in the epic dependency graph.
func SortEpicsByDependency(breakdown *EpicBreakdown) ([]string, error) {
	if breakdown == nil || len(breakdown.Epics) == 0 {
		return nil, nil
	}

	// Build a set of known epic IDs for quick lookup.
	epicSet := make(map[string]bool, len(breakdown.Epics))
	for _, e := range breakdown.Epics {
		epicSet[e.ID] = true
	}

	// Build in-degree map and adjacency list (dep -> dependents).
	inDegree := make(map[string]int, len(breakdown.Epics))
	// dependents[dep] is the list of epic IDs that depend on dep.
	dependents := make(map[string][]string, len(breakdown.Epics))

	for _, e := range breakdown.Epics {
		// Ensure every epic has an entry in inDegree.
		if _, ok := inDegree[e.ID]; !ok {
			inDegree[e.ID] = 0
		}
		for _, dep := range e.DependenciesOnEpics {
			// Only count dependencies that are within the breakdown; unknown deps
			// are ignored here (schema validation would have caught them earlier).
			if epicSet[dep] {
				inDegree[e.ID]++
				dependents[dep] = append(dependents[dep], e.ID)
			}
		}
	}

	// Seed the queue with all epics that have in-degree 0, sorted for determinism.
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)

	var ordered []string
	for len(queue) > 0 {
		// Pop the first element (already sorted).
		current := queue[0]
		queue = queue[1:]
		ordered = append(ordered, current)

		// Decrement in-degree of all dependents.
		var newZero []string
		for _, dependent := range dependents[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				newZero = append(newZero, dependent)
			}
		}
		// Sort newly zero-degree epics before inserting into queue for determinism.
		sort.Strings(newZero)
		queue = append(queue, newZero...)
		// Re-sort the queue to maintain deterministic ordering when multiple
		// epics become available simultaneously.
		sort.Strings(queue)
	}

	if len(ordered) != len(breakdown.Epics) {
		// Some epics were not processed — they form a cycle.
		var cycle []string
		for id, deg := range inDegree {
			if deg > 0 {
				cycle = append(cycle, id)
			}
		}
		sort.Strings(cycle)
		return nil, fmt.Errorf("cyclic epic dependency detected: %v form a cycle", cycle)
	}

	return ordered, nil
}

// AssignGlobalIDs assigns T-001, T-002, ... to all tasks across epics in the
// order determined by epicOrder. Tasks within each epic retain their original order.
//
// Epics present in results but absent from epicOrder are appended at the end,
// sorted by epic ID, for deterministic output.
// Epics present in epicOrder but absent from results are silently skipped.
//
// The zero-padding width is 3 digits for fewer than 1000 total tasks, or 4 digits
// for 1000 or more.
//
// Returns the merged tasks with global IDs assigned and the ID mapping from
// temp_id to global_id.
func AssignGlobalIDs(
	epicOrder []string,
	results map[string]*EpicTaskResult,
) ([]MergedTask, IDMapping) {
	// Count total tasks to determine zero-padding width.
	total := 0
	for _, etr := range results {
		total += len(etr.Tasks)
	}

	format := "T-%03d"
	if total >= 1000 {
		format = "T-%04d"
	}

	// Build the final ordered list of epic IDs to process.
	// Start with the topologically sorted order, skipping epics not in results.
	seen := make(map[string]bool, len(epicOrder))
	finalOrder := make([]string, 0, len(epicOrder))
	for _, id := range epicOrder {
		if _, ok := results[id]; ok {
			finalOrder = append(finalOrder, id)
			seen[id] = true
		}
	}

	// Append epics in results but not in epicOrder (sorted by ID for determinism).
	var extras []string
	for id := range results {
		if !seen[id] {
			extras = append(extras, id)
		}
	}
	sort.Strings(extras)
	finalOrder = append(finalOrder, extras...)

	// Assign sequential global IDs.
	merged := make([]MergedTask, 0, total)
	mapping := make(IDMapping, total)

	counter := 1
	for _, epicID := range finalOrder {
		etr := results[epicID]
		for _, task := range etr.Tasks {
			if task.TempID == "" {
				// Skip tasks with empty temp IDs; they are invalid but we are
				// lenient here since validation should have caught them earlier.
				continue
			}
			globalID := fmt.Sprintf(format, counter)
			counter++

			mapping[task.TempID] = globalID

			merged = append(merged, MergedTask{
				GlobalID:           globalID,
				TempID:             task.TempID,
				EpicID:             epicID,
				Title:              task.Title,
				Description:        task.Description,
				AcceptanceCriteria: task.AcceptanceCriteria,
				LocalDependencies:  task.LocalDependencies,
				CrossEpicDeps:      task.CrossEpicDeps,
				Effort:             task.Effort,
				Priority:           task.Priority,
			})
		}
	}

	return merged, mapping
}
