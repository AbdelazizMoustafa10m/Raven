# T-061: Merge Phase -- Dependency Remapping (Local to Global IDs)

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 2-4hrs |
| Dependencies | T-056, T-060 |
| Blocked By | T-060 |
| Blocks | T-062, T-063 |

## Goal
Implement the second step of the merge phase: remap all task dependencies from temporary per-epic IDs (e.g., `E001-T02`) and cross-epic references (e.g., `E-003:database-schema`) to their assigned global IDs (e.g., `T-007`). This produces a unified dependency graph expressed entirely in global task IDs.

## Background
Per PRD Section 5.8, Phase 3 step 2 is: "Remaps local and cross-epic dependencies to global IDs." During the scatter phase, each worker produces tasks with two types of dependencies: (1) `local_dependencies` referencing tasks within the same epic using temp IDs like `E001-T02`, and (2) `cross_epic_dependencies` referencing tasks in other epics using the format `E-003:label`. The IDMapping from T-060 provides the temp_id -> global_id translation. Cross-epic dependencies need a fuzzy match by label against task titles in the target epic.

## Technical Specifications
### Implementation Approach
Create a `RemapDependencies` function in `internal/prd/merger.go` that takes the slice of `MergedTask` and the `IDMapping`, then rewrites every task's dependency lists from temp IDs to global IDs. For local dependencies, this is a direct lookup in IDMapping. For cross-epic dependencies, parse the `E-NNN:label` format, find tasks in the target epic, and match by normalized title similarity.

### Key Components
- **RemapDependencies**: Main function that transforms all dependency references
- **CrossEpicResolver**: Resolves `E-NNN:label` references to global task IDs by fuzzy title matching
- **RemapReport**: Summary of successful remaps, unresolved references, and ambiguous matches

### API/Interface Contracts
```go
// internal/prd/merger.go (continued)

type RemapReport struct {
    Remapped   int               // Successfully remapped references
    Unresolved []UnresolvedRef   // References that could not be resolved
    Ambiguous  []AmbiguousRef    // References with multiple possible matches
}

type UnresolvedRef struct {
    TaskID    string // Global ID of the task with the unresolved dep
    Reference string // The original temp_id or cross-epic ref
}

type AmbiguousRef struct {
    TaskID     string   // Global ID of the task with the ambiguous dep
    Reference  string   // The original cross-epic ref
    Candidates []string // Multiple global IDs that matched
}

// RemapDependencies rewrites all task dependencies from temp IDs to global IDs.
// Returns the updated tasks and a report of the remapping process.
func RemapDependencies(
    tasks []MergedTask,
    idMapping IDMapping,
    epicTasks map[string][]MergedTask, // epicID -> tasks in that epic
) ([]MergedTask, *RemapReport)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/prd (T-056, T-060) | - | MergedTask, IDMapping types |
| strings | stdlib | Title normalization for fuzzy matching |
| unicode | stdlib | Unicode-aware string normalization |

## Acceptance Criteria
- [ ] Local dependencies (`E001-T02`) are remapped to global IDs via direct IDMapping lookup
- [ ] Cross-epic dependencies (`E-003:database-schema`) are resolved by matching label against task titles in target epic
- [ ] Title matching is case-insensitive and ignores leading/trailing whitespace
- [ ] Unresolved references are collected in RemapReport (not a fatal error)
- [ ] Ambiguous references (multiple title matches) are collected in RemapReport
- [ ] Self-dependencies (task depending on itself) are removed with a warning
- [ ] Final task dependency lists contain only global IDs (T-NNN format)
- [ ] Unit tests achieve 95% coverage

## Testing Requirements
### Unit Tests
- Local dependency `E001-T02` maps to `T-005` via IDMapping
- Cross-epic dependency `E-003:Set up database schema` matches task titled "Set up database schema" in E-003
- Cross-epic dependency with different casing still matches
- Unresolved local dependency (ID not in mapping) reported in RemapReport
- Unresolved cross-epic dependency (no title match in target epic) reported
- Ambiguous cross-epic dependency (two tasks with similar titles) reported
- Self-dependency removed
- Task with no dependencies remains unchanged
- Multiple dependencies in one task all remapped correctly

### Edge Cases to Handle
- Cross-epic label that partially matches multiple tasks
- Dependencies on tasks in epics that had 0 tasks (empty epics)
- Circular local references within same epic (handled by DAG validation in T-063)
- Very long task titles (>200 chars) in cross-epic references

## Implementation Notes
### Recommended Approach
1. Build a reverse lookup: for each epic, index tasks by normalized title
2. For each task, iterate through `LocalDependencies`:
   - Look up temp_id in IDMapping -- if found, replace; if not, add to unresolved
3. For each task, iterate through `CrossEpicDeps`:
   - Parse `E-NNN:label` format (split on first `:`)
   - Find tasks in target epic with normalized title matching the label
   - If exactly one match, replace with its global ID
   - If zero matches, add to unresolved
   - If multiple matches, add to ambiguous (use first match as best guess)
4. Merge local and cross-epic dependencies into a single `Dependencies []string` field
5. Remove any self-references
6. Return updated tasks and report

### Potential Pitfalls
- Cross-epic dependency format parsing: the label part may itself contain colons -- split on first colon only
- Title normalization: consider stripping common prefixes like "Implement", "Set up", "Create" for better matching
- Duplicate dependencies after remapping (same global ID from both local and cross-epic) -- deduplicate

### Security Considerations
- None specific to this task (pure data transformation)

## References
- [PRD Section 5.8 - Phase 3 Gather, step 2](docs/prd/PRD-Raven.md)
