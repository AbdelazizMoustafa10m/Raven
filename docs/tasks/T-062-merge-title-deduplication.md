# T-062: Merge Phase -- Title Deduplication

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-060, T-061 |
| Blocked By | T-061 |
| Blocks | T-064 |

## Goal
Implement the third step of the merge phase (Phase 3, step 3): deduplicate tasks across epics by normalized title similarity. When the same logical task appears in multiple epics (a common LLM artifact during parallel decomposition), merge them into a single task, combining acceptance criteria and preserving the instance from the earlier epic. Return a detailed deduplication report for user review.

## Background
Per PRD Section 5.8, Phase 3 step 3 is: "Deduplicates by title similarity (normalized string comparison)." During the scatter phase, independent LLM workers may produce overlapping tasks -- for example, both an "Authentication" epic and a "Database" epic might generate a "Set up database connection pool" task. The deduplication step detects these near-duplicates using normalized string comparison and merges them, preventing redundant work items in the final task set.

This step operates on the `[]MergedTask` produced by T-060 (global ID assignment) after dependency remapping by T-061. It must be careful to update all dependency references when removing duplicate tasks, so that no task depends on a removed duplicate.

## Technical Specifications
### Implementation Approach
Create deduplication functions in `internal/prd/merger.go` that operate on the merged task list. Normalization involves: lowercasing, stripping common action-verb prefixes ("implement", "create", "set up", "add", "build", "define", "write", "configure"), collapsing whitespace, and removing punctuation. Two tasks are considered duplicates if their normalized titles are identical. When a duplicate pair is found, the task from the earlier epic (lower global ID) is kept, and the later one's acceptance criteria are merged into it. All dependency references to the removed task's global ID are rewritten to point to the kept task's global ID.

### Key Components
- **NormalizeTitle**: Pure function that normalizes a task title for comparison
- **FindDuplicates**: Scans the task list and groups tasks by normalized title
- **DeduplicateTasks**: Merges duplicate groups and rewrites dependency references
- **DedupReport**: Summary of removed tasks, merged criteria, and rewritten dependencies

### API/Interface Contracts
```go
// internal/prd/merger.go (continued)

// NormalizeTitle returns a normalized version of a task title for dedup comparison.
// Lowercase, strip common prefixes, collapse whitespace, remove punctuation.
func NormalizeTitle(title string) string

// DedupGroup represents a set of tasks with matching normalized titles.
type DedupGroup struct {
    NormalizedTitle string
    Tasks           []MergedTask // ordered by GlobalID (earliest first)
}

// DedupReport summarizes the deduplication results.
type DedupReport struct {
    OriginalCount  int            // Total tasks before dedup
    RemovedCount   int            // Number of tasks removed
    FinalCount     int            // Total tasks after dedup
    Merges         []DedupMerge   // Details of each merge operation
    RewrittenDeps  int            // Number of dependency references rewritten
}

// DedupMerge describes a single merge operation.
type DedupMerge struct {
    KeptTaskID      string   // Global ID of the kept task
    KeptTitle       string   // Original title of kept task
    RemovedTaskIDs  []string // Global IDs of removed duplicates
    RemovedTitles   []string // Original titles of removed duplicates
    MergedCriteria  int      // Number of acceptance criteria merged in
}

// DeduplicateTasks removes duplicate tasks by normalized title similarity.
// Returns the deduplicated task list and a report.
// The kept task is always the one with the lowest global ID (earliest epic).
// Acceptance criteria from removed tasks are appended to the kept task.
// All dependency references to removed tasks are rewritten to the kept task.
func DeduplicateTasks(tasks []MergedTask) ([]MergedTask, *DedupReport)

// findDuplicateGroups groups tasks by normalized title.
// Only returns groups with 2+ tasks (actual duplicates).
func findDuplicateGroups(tasks []MergedTask) []DedupGroup
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/prd (T-060) | - | MergedTask type |
| strings | stdlib | String normalization (ToLower, TrimSpace, Fields) |
| unicode | stdlib | Punctuation detection and removal |
| regexp | stdlib | Common prefix stripping patterns |

## Acceptance Criteria
- [ ] NormalizeTitle lowercases, strips common prefixes, collapses whitespace, and removes punctuation
- [ ] Common prefixes stripped include: "implement", "create", "set up", "add", "build", "define", "write", "configure", "design", "establish"
- [ ] Tasks with identical normalized titles are detected as duplicates
- [ ] The task with the lowest global ID (from the earliest epic) is kept
- [ ] Acceptance criteria from removed tasks are appended to the kept task (no duplicates in merged criteria)
- [ ] All dependency references to removed task IDs are rewritten to the kept task's ID
- [ ] Self-dependencies created by rewriting are removed
- [ ] DedupReport accurately reflects all merge operations
- [ ] Tasks with unique titles are untouched
- [ ] Unit tests achieve 95% coverage

## Testing Requirements
### Unit Tests
- NormalizeTitle("Implement user authentication") == NormalizeTitle("Create user authentication")
- NormalizeTitle("Set up database connection") == NormalizeTitle("set up database connection")
- NormalizeTitle("Build CI/CD pipeline") strips "Build" prefix
- NormalizeTitle preserves meaningful words after prefix stripping
- NormalizeTitle("  Multiple   Spaces  ") collapses to "multiple spaces"
- NormalizeTitle("Add: logging middleware!") removes punctuation
- Two tasks with identical normalized titles: earlier one kept, later removed
- Three tasks with same normalized title: earliest kept, other two removed
- Acceptance criteria merged without duplicates
- Dependency on removed task rewritten to kept task's ID
- Task depending on itself after rewrite: self-dep removed
- No duplicates in task list: returns identical list with empty DedupReport
- Single task in list: returns unchanged
- Tasks with similar but not identical normalized titles: both kept (no false positives)

### Integration Tests
- Full dedup pipeline with realistic task set (10+ tasks, 2-3 duplicate groups)
- Dedup followed by dependency validation (no dangling references after dedup)

### Edge Cases to Handle
- Task title is entirely a prefix word (e.g., "Implement") -- after stripping, empty string; should keep original
- Very long titles (>500 chars) -- normalization should still work
- Unicode titles (e.g., accented characters in descriptions)
- Acceptance criteria that are identical between duplicates -- should not produce double entries after merge
- Circular dependency chains involving a removed task -- rewriting should maintain chain integrity
- Task with no acceptance criteria merged with task that has criteria
- All tasks are duplicates of each other (pathological case)

## Implementation Notes
### Recommended Approach
1. Build a map of normalized title -> []MergedTask
2. For each group with len > 1, select the keeper (lowest GlobalID)
3. For each removed task in the group:
   a. Append its unique acceptance criteria to the keeper
   b. Record the mapping: removed_id -> keeper_id
4. Build a removal set (set of removed global IDs)
5. Walk all remaining tasks' dependency lists:
   a. Replace any removed_id with the corresponding keeper_id
   b. Deduplicate the dependency list (a task may now depend on the same keeper twice)
   c. Remove self-references
6. Filter out removed tasks from the final list
7. Build and return DedupReport

### Potential Pitfalls
- Prefix stripping must be word-boundary aware: "Implement" should strip from "Implement user auth" but NOT alter "Implementation details" -- use word boundary regex or check for space/end-of-string after the prefix
- When merging acceptance criteria, use a set (map[string]bool) to avoid duplicates, but preserve ordering from the keeper first
- Global IDs are NOT re-sequenced after dedup -- gaps are acceptable (T-001, T-002, T-004 if T-003 was removed). Re-sequencing is a separate concern handled by the emitter if needed
- Dependency rewriting must be done on ALL remaining tasks, not just the keeper -- any task in the entire list may depend on a removed duplicate

### Security Considerations
- None specific to this task (pure data transformation)

## References
- [PRD Section 5.8 - Phase 3 Gather, step 3](docs/prd/PRD-Raven.md)
- [adrg/strutil - Go string metrics library](https://github.com/adrg/strutil)
- [Go strings package](https://pkg.go.dev/strings)
- [Go unicode package](https://pkg.go.dev/unicode)