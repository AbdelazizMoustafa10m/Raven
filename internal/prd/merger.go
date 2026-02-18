package prd

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
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

// DedupGroup represents a set of tasks with matching normalized titles.
type DedupGroup struct {
	// NormalizedTitle is the shared normalized form used for deduplication matching.
	NormalizedTitle string
	// Tasks holds the tasks in this group, ordered by GlobalID (earliest first).
	Tasks []MergedTask
}

// DedupReport summarizes the deduplication results.
type DedupReport struct {
	// OriginalCount is the total number of tasks before deduplication.
	OriginalCount int
	// RemovedCount is the number of tasks removed as duplicates.
	RemovedCount int
	// FinalCount is the total number of tasks after deduplication.
	FinalCount int
	// Merges describes each merge operation performed.
	Merges []DedupMerge
	// RewrittenDeps is the number of dependency references rewritten to point to keeper tasks.
	RewrittenDeps int
}

// DedupMerge describes a single merge operation where one or more duplicate tasks
// were merged into a keeper task.
type DedupMerge struct {
	// KeptTaskID is the global ID of the task that was kept.
	KeptTaskID string
	// KeptTitle is the original title of the kept task.
	KeptTitle string
	// RemovedTaskIDs lists the global IDs of the tasks that were removed.
	RemovedTaskIDs []string
	// RemovedTitles lists the original titles of the removed tasks.
	RemovedTitles []string
	// MergedCriteria is the number of acceptance criteria merged in from removed tasks.
	MergedCriteria int
}

// actionPrefixes lists the common action-verb prefixes to strip during normalization.
// Multi-word prefixes (e.g., "set up") must appear before their single-word prefix
// sub-strings to ensure the longer match is attempted first.
var actionPrefixes = []string{
	"set up",
	"implement",
	"create",
	"add",
	"build",
	"define",
	"write",
	"configure",
	"design",
	"establish",
}

// rePunct matches any rune that is not a letter, digit, or space.
var rePunct = regexp.MustCompile(`[^\p{L}\p{N} ]+`)

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

// NormalizeTitle returns a normalized version of a task title for deduplication comparison.
// Steps applied in order:
//  1. Lowercase the entire string.
//  2. Strip common action-verb prefixes (word-boundary aware -- only strips when the prefix
//     is followed by a space or end-of-string, not by another letter).
//  3. Collapse multiple consecutive spaces into one and trim leading/trailing space.
//  4. Remove all punctuation (non-alphanumeric, non-space characters).
//
// If the result after stripping is empty, the original lowercased+normalized title is
// returned as a fallback (e.g. when the title is itself the prefix word, like "Implement").
func NormalizeTitle(title string) string {
	// Step 1: lowercase.
	s := strings.ToLower(title)

	// Step 2: strip action-verb prefixes with word-boundary awareness.
	// A prefix is only stripped when it is followed by a space or is the entire string.
	for _, prefix := range actionPrefixes {
		if !strings.HasPrefix(s, prefix) {
			continue
		}
		rest := s[len(prefix):]
		// Word-boundary check: rest must be empty or start with a space.
		if rest == "" || (len(rest) > 0 && rest[0] == ' ') {
			candidate := strings.TrimSpace(rest)
			if candidate != "" {
				s = candidate
			}
			// Only strip the first matching prefix.
			break
		}
	}

	// Step 3: collapse whitespace.
	fields := strings.FieldsFunc(s, unicode.IsSpace)
	s = strings.Join(fields, " ")

	// Step 4: remove punctuation.
	s = rePunct.ReplaceAllString(s, "")

	// Collapse any spaces introduced or left by punctuation removal.
	fields = strings.Fields(s)
	s = strings.Join(fields, " ")

	return s
}

// findDuplicateGroups groups tasks by normalized title and returns only groups with
// two or more tasks (actual duplicates). Within each group, tasks are sorted by GlobalID
// so the earliest-assigned task (lowest GlobalID) is first.
func findDuplicateGroups(tasks []MergedTask) []DedupGroup {
	// Map normalized title -> tasks in insertion order.
	index := make(map[string][]MergedTask, len(tasks))
	order := make([]string, 0, len(tasks))

	for _, task := range tasks {
		norm := NormalizeTitle(task.Title)
		if _, exists := index[norm]; !exists {
			order = append(order, norm)
		}
		index[norm] = append(index[norm], task)
	}

	var groups []DedupGroup
	for _, norm := range order {
		group := index[norm]
		if len(group) < 2 {
			continue
		}
		// Sort by GlobalID so the keeper (lowest ID) is first.
		sort.Slice(group, func(i, j int) bool {
			return group[i].GlobalID < group[j].GlobalID
		})
		groups = append(groups, DedupGroup{
			NormalizedTitle: norm,
			Tasks:           group,
		})
	}
	return groups
}

// DeduplicateTasks removes duplicate tasks that share a normalized title. The task with
// the lowest GlobalID in each duplicate group is kept; all others are removed. Unique
// acceptance criteria from removed tasks are appended to the keeper's criteria list.
// All dependency references that pointed to removed tasks are rewritten to reference the
// keeper instead, and self-references are dropped. The final task list preserves the
// original ordering of the keeper tasks.
//
// Returns the deduplicated task list and a DedupReport summarising what was done.
func DeduplicateTasks(tasks []MergedTask) ([]MergedTask, *DedupReport) {
	report := &DedupReport{
		OriginalCount: len(tasks),
	}

	if len(tasks) == 0 {
		return nil, report
	}

	// Build duplicate groups.
	groups := findDuplicateGroups(tasks)

	if len(groups) == 0 {
		// No duplicates: return a copy with zero-value report fields set.
		out := make([]MergedTask, len(tasks))
		copy(out, tasks)
		report.FinalCount = len(out)
		return out, report
	}

	// removedToKeeper maps removed global ID -> keeper global ID.
	removedToKeeper := make(map[string]string)

	// keeperUpdates maps keeper global ID -> updated MergedTask (with merged criteria).
	keeperUpdates := make(map[string]MergedTask)

	for _, group := range groups {
		keeper := group.Tasks[0]

		// Defensively copy the AcceptanceCriteria slice so we do not mutate the
		// original input's backing array when appending new criteria (DC-1).
		copiedAC := make([]string, len(keeper.AcceptanceCriteria))
		copy(copiedAC, keeper.AcceptanceCriteria)
		keeper.AcceptanceCriteria = copiedAC

		// Build a set of existing acceptance criteria for the keeper to avoid duplicates.
		existingCriteria := make(map[string]bool, len(keeper.AcceptanceCriteria))
		for _, ac := range keeper.AcceptanceCriteria {
			existingCriteria[ac] = true
		}

		var mergedCount int
		var removedIDs []string
		var removedTitles []string

		for _, dup := range group.Tasks[1:] {
			removedToKeeper[dup.GlobalID] = keeper.GlobalID
			removedIDs = append(removedIDs, dup.GlobalID)
			removedTitles = append(removedTitles, dup.Title)

			// Merge unique acceptance criteria from the removed task.
			for _, ac := range dup.AcceptanceCriteria {
				if !existingCriteria[ac] {
					existingCriteria[ac] = true
					keeper.AcceptanceCriteria = append(keeper.AcceptanceCriteria, ac)
					mergedCount++
				}
			}
		}

		keeperUpdates[keeper.GlobalID] = keeper

		report.Merges = append(report.Merges, DedupMerge{
			KeptTaskID:     keeper.GlobalID,
			KeptTitle:      keeper.Title,
			RemovedTaskIDs: removedIDs,
			RemovedTitles:  removedTitles,
			MergedCriteria: mergedCount,
		})
		report.RemovedCount += len(removedIDs)
	}

	// Build a set of removed IDs for quick lookup during the filter pass.
	removedSet := make(map[string]bool, report.RemovedCount)
	for removedID := range removedToKeeper {
		removedSet[removedID] = true
	}

	// Walk all tasks: apply keeper criteria updates, rewrite dependencies, and filter
	// out removed tasks. The output preserves the original order of keeper tasks.
	out := make([]MergedTask, 0, len(tasks)-report.RemovedCount)

	for _, task := range tasks {
		if removedSet[task.GlobalID] {
			// This task was removed; skip it.
			continue
		}

		// Apply accumulated acceptance-criteria merges for keeper tasks.
		if updated, ok := keeperUpdates[task.GlobalID]; ok {
			task = updated
		}

		// Rewrite dependency references.
		if len(task.Dependencies) > 0 {
			seen := make(map[string]bool, len(task.Dependencies))
			rewritten := make([]string, 0, len(task.Dependencies))

			for _, dep := range task.Dependencies {
				// Rewrite removed IDs to their keeper.
				if keeperID, wasRemoved := removedToKeeper[dep]; wasRemoved {
					report.RewrittenDeps++
					dep = keeperID
				}

				// Skip self-references.
				if dep == task.GlobalID {
					continue
				}

				// Deduplicate.
				if !seen[dep] {
					seen[dep] = true
					rewritten = append(rewritten, dep)
				}
			}

			task.Dependencies = rewritten
		}

		out = append(out, task)
	}

	report.FinalCount = len(out)
	return out, report
}
