package prd

import (
	"fmt"
	"sort"
	"strings"
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
	// Dependencies contains the resolved global task IDs after dependency remapping.
	// Populated by RemapDependencies; empty until that step runs.
	Dependencies []string
	// Effort is the size estimate; one of: "small", "medium", "large".
	Effort string
	// Priority is the importance classification; one of: "must-have", "should-have", "nice-to-have".
	Priority string
}

// RemapReport summarizes the results of a dependency remapping operation.
type RemapReport struct {
	// Remapped is the count of dependency references that were successfully resolved.
	Remapped int
	// Unresolved holds references that could not be mapped to any global ID.
	Unresolved []UnresolvedRef
	// Ambiguous holds cross-epic references that matched more than one task title.
	Ambiguous []AmbiguousRef
}

// UnresolvedRef records a dependency reference that could not be mapped to a global ID.
type UnresolvedRef struct {
	// TaskID is the global ID of the task that contains the unresolved dependency.
	TaskID string
	// Reference is the original temp_id or cross-epic ref that could not be resolved.
	Reference string
}

// AmbiguousRef records a cross-epic dependency reference that matched multiple tasks.
type AmbiguousRef struct {
	// TaskID is the global ID of the task that contains the ambiguous dependency.
	TaskID string
	// Reference is the original cross-epic ref (e.g., "E-003:some-label").
	Reference string
	// Candidates lists all global IDs whose title matched the label.
	Candidates []string
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

// RemapDependencies rewrites all task dependencies from temp IDs to global IDs.
// It processes both LocalDependencies (intra-epic temp_id references) and CrossEpicDeps
// ("E-NNN:label" references). The resolved global IDs are merged, deduplicated, and stored
// in each task's Dependencies field.
//
// The epicTasks parameter maps an epic ID to the list of MergedTask values belonging to
// that epic; callers typically build this from the same tasks slice grouped by EpicID.
//
// Returns the updated tasks and a report summarising how many references were resolved,
// which could not be resolved, and which were ambiguous.
func RemapDependencies(
	tasks []MergedTask,
	idMapping IDMapping,
	epicTasks map[string][]MergedTask,
) ([]MergedTask, *RemapReport) {
	report := &RemapReport{}

	// Build a per-epic title index: epicID -> normalised_title -> globalID.
	// This is used when resolving cross-epic deps by label.
	titleIndex := make(map[string]map[string]string, len(epicTasks))
	for epicID, epicTaskList := range epicTasks {
		idx := make(map[string]string, len(epicTaskList))
		for _, t := range epicTaskList {
			norm := strings.TrimSpace(strings.ToLower(t.Title))
			if norm != "" {
				idx[norm] = t.GlobalID
			}
		}
		titleIndex[epicID] = idx
	}

	updated := make([]MergedTask, len(tasks))
	for i, task := range tasks {
		// seen tracks global IDs already added to Dependencies for this task,
		// preventing duplicates that arise when the same task is referenced by
		// both a local dep and a cross-epic dep.
		seen := make(map[string]bool)
		var deps []string

		// --- Resolve LocalDependencies (temp_id -> global_id) ---
		for _, ref := range task.LocalDependencies {
			globalID, ok := idMapping[ref]
			if !ok {
				report.Unresolved = append(report.Unresolved, UnresolvedRef{
					TaskID:    task.GlobalID,
					Reference: ref,
				})
				continue
			}

			// Skip self-references.
			if globalID == task.GlobalID {
				continue
			}

			if !seen[globalID] {
				seen[globalID] = true
				deps = append(deps, globalID)
				report.Remapped++
			}
		}

		// --- Resolve CrossEpicDeps ("E-NNN:label" -> global_id) ---
		for _, ref := range task.CrossEpicDeps {
			// Split on the FIRST colon only to preserve colons in labels.
			parts := strings.SplitN(ref, ":", 2)
			if len(parts) != 2 {
				// Malformed reference; treat as unresolved.
				report.Unresolved = append(report.Unresolved, UnresolvedRef{
					TaskID:    task.GlobalID,
					Reference: ref,
				})
				continue
			}

			targetEpicID := parts[0]
			label := strings.TrimSpace(strings.ToLower(parts[1]))

			epicIdx, epicFound := titleIndex[targetEpicID]
			if !epicFound {
				report.Unresolved = append(report.Unresolved, UnresolvedRef{
					TaskID:    task.GlobalID,
					Reference: ref,
				})
				continue
			}

			// Search for tasks in the target epic whose normalised title contains
			// the normalised label (substring match to handle slug vs full title).
			var matches []string
			for normTitle, globalID := range epicIdx {
				if normTitle == label || strings.Contains(normTitle, label) || strings.Contains(label, normTitle) {
					matches = append(matches, globalID)
				}
			}
			sort.Strings(matches) // deterministic ordering of candidates

			switch len(matches) {
			case 0:
				report.Unresolved = append(report.Unresolved, UnresolvedRef{
					TaskID:    task.GlobalID,
					Reference: ref,
				})
			case 1:
				globalID := matches[0]
				// Skip self-references.
				if globalID == task.GlobalID {
					continue
				}
				if !seen[globalID] {
					seen[globalID] = true
					deps = append(deps, globalID)
					report.Remapped++
				}
			default:
				// Multiple matches — record the ambiguity and use the first candidate
				// as the best guess so the output remains usable.
				report.Ambiguous = append(report.Ambiguous, AmbiguousRef{
					TaskID:     task.GlobalID,
					Reference:  ref,
					Candidates: matches,
				})
				best := matches[0]
				if best != task.GlobalID && !seen[best] {
					seen[best] = true
					deps = append(deps, best)
					report.Remapped++
				}
			}
		}

		// Assign the resolved dependencies back to the task copy.
		task.Dependencies = deps
		updated[i] = task
	}

	return updated, report
}
