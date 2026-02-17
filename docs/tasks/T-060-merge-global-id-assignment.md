# T-060: Merge Phase -- Global Sequential ID Assignment

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 2-4hrs |
| Dependencies | T-056 |
| Blocked By | T-056 |
| Blocks | T-061, T-062, T-063 |

## Goal
Implement the first step of the merge phase (Phase 3): assign global sequential task IDs (`T-001`, `T-002`, ...) across all epics, ordered by epic dependency topology. This transforms scattered per-epic temporary IDs into a unified global numbering scheme.

## Background
Per PRD Section 5.8, Phase 3 (Gather) is deterministic with no LLM calls. Step 1 is: "Assigns global sequential IDs (T-001, T-002, ...) across epics, ordered by epic dependency." The epics are ordered by their declared `dependencies_on_epics` field -- epics with no dependencies come first, then epics that depend on them, etc. Within each epic, tasks retain their relative order from the worker output. This ordering ensures that dependency references are always forward-pointing (a task can depend on tasks with lower IDs).

## Technical Specifications
### Implementation Approach
Create `internal/prd/merger.go` with a function that takes all `EpicTaskResult` slices and the `EpicBreakdown` (for epic dependency ordering). First, topologically sort the epics by their inter-epic dependencies. Then, iterate through epics in topological order, assigning sequential global IDs to each task. Build a mapping from temporary IDs (`E001-T01`) to global IDs (`T-001`) for use by the dependency remapping step (T-061).

### Key Components
- **EpicSorter**: Topologically sorts epics by their dependency declarations
- **IDAssigner**: Walks sorted epics and assigns sequential global IDs
- **IDMapping**: Map from temp_id to global_id for downstream remapping

### API/Interface Contracts
```go
// internal/prd/merger.go

type IDMapping map[string]string // temp_id -> global_id, e.g., "E001-T01" -> "T-001"

type MergedTask struct {
    GlobalID           string   // T-001 format
    TempID             string   // Original E001-T01
    EpicID             string   // Source epic
    Title              string
    Description        string
    AcceptanceCriteria []string
    LocalDependencies  []string // Still temp IDs at this point
    CrossEpicDeps      []string // Still temp IDs at this point
    Effort             string
    Priority           string
}

// SortEpicsByDependency returns epic IDs in topological order.
// Returns error if there is a cycle in epic dependencies.
func SortEpicsByDependency(breakdown *EpicBreakdown) ([]string, error)

// AssignGlobalIDs assigns T-001, T-002, ... to all tasks across epics
// in the order determined by epic topology.
// Returns the merged tasks with global IDs and the ID mapping.
func AssignGlobalIDs(
    epicOrder []string,
    results map[string]*EpicTaskResult,
) ([]MergedTask, IDMapping)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/prd (T-056) | - | EpicBreakdown and EpicTaskResult types |
| fmt | stdlib | ID formatting (T-%03d) |

## Acceptance Criteria
- [ ] Epics are sorted topologically by `dependencies_on_epics` field
- [ ] Tasks within each epic retain their original order
- [ ] Global IDs are sequential starting at T-001 with zero-padded three-digit format
- [ ] IDMapping correctly maps every temp_id to its global_id
- [ ] Cyclic epic dependencies produce a clear error message
- [ ] Works correctly with epics that have no dependencies (placed first)
- [ ] Unit tests achieve 95% coverage

## Testing Requirements
### Unit Tests
- 3 epics with linear dependency chain: E-001 -> E-002 -> E-003, tasks numbered correctly
- 3 epics with no dependencies: all valid orderings accepted, IDs sequential
- Diamond dependency: E-001, E-002 depends on E-001, E-003 depends on E-001, E-004 depends on E-002 and E-003
- Cyclic epic dependency returns error
- Single epic with 5 tasks: T-001 through T-005
- IDMapping contains all temp_id -> global_id entries
- Zero tasks in one epic: skipped, no gap in numbering

### Edge Cases to Handle
- Epic with 0 tasks (empty epic after scatter)
- Very large number of tasks (1000+) -- ID format should still be T-001 (3 digits sufficient for <1000, extend to T-0001 if >=1000)
- Epic dependency on self

## Implementation Notes
### Recommended Approach
1. Build adjacency list from epic dependencies
2. Run Kahn's algorithm: find epics with in-degree 0, process them, decrement dependents' in-degrees
3. If not all epics processed, there is a cycle -- return error with cycle details
4. Iterate through sorted epics, for each epic iterate through tasks, assign T-NNN
5. Use `fmt.Sprintf("T-%03d", counter)` for ID formatting (or T-%04d if total tasks >= 1000)
6. Build IDMapping as you go

### Potential Pitfalls
- Zero-padding width: PRD uses T-001 (3 digits). For projects with >999 tasks, need 4 digits. Determine width based on total task count before assigning IDs
- Epic dependency format: `dependencies_on_epics` contains epic IDs like "E-001" -- match against `Epic.ID` field
- Kahn's algorithm needs careful handling of the "remaining nodes" check for cycle detection

### Security Considerations
- None specific to this task (pure data transformation)

## References
- [PRD Section 5.8 - Phase 3 Gather, step 1](docs/prd/PRD-Raven.md)
- [Kahn's algorithm for topological sorting](https://en.wikipedia.org/wiki/Topological_sorting#Kahn's_algorithm)
- [gammazero/toposort Go package](https://pkg.go.dev/github.com/gammazero/toposort)
