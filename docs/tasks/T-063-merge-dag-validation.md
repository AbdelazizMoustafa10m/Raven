# T-063: Merge Phase -- DAG Validation

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-060, T-061 |
| Blocked By | T-061 |
| Blocks | T-064 |

## Goal
Implement the fourth step of the merge phase (Phase 3, step 4): validate that the final task dependency graph is a Directed Acyclic Graph (DAG) using Kahn's algorithm for topological sorting with cycle detection. Also validate referential integrity -- no references to nonexistent tasks and no self-references. Produce clear, actionable error reports when validation fails, including the exact tasks forming any detected cycle.

## Background
Per PRD Section 5.8, Phase 3 step 4 is: "Validates the dependency graph is a DAG (using topological sort; cycle detection)." After global ID assignment (T-060), dependency remapping (T-061), and deduplication (T-062), the merged task list has a dependency graph expressed entirely in global IDs. This graph must be validated as a DAG before task files can be emitted -- a cycle in the dependency graph would make it impossible to determine execution order. The PRD specifies using Kahn's algorithm (BFS-based topological sort) which naturally detects cycles: if not all nodes are processed after the algorithm completes, the remaining nodes are part of one or more cycles.

Additionally, this step computes the topological depth of each task (longest path from a root node), which T-064 (emitter) uses to auto-generate `phases.conf`. Tasks at depth 0 have no dependencies and belong to the earliest phase; tasks at depth N depend (transitively) on tasks at lower depths.

## Technical Specifications
### Implementation Approach
Create DAG validation functions in `internal/prd/merger.go` that take the final `[]MergedTask` and perform three checks: (1) referential integrity -- all dependency IDs exist in the task list, (2) no self-references, and (3) the graph is a DAG via Kahn's algorithm. Kahn's algorithm processes nodes with in-degree 0, removes their outgoing edges, and repeats. If nodes remain unprocessed, they form cycles. Additionally, compute topological depth (the longest path from any root to each node) for phase assignment.

### Key Components
- **ValidateDAG**: Main validation entry point returning structured results
- **buildAdjacency**: Constructs adjacency list and in-degree map from task dependencies
- **kahnSort**: Kahn's algorithm implementation returning topological order and any cycle
- **computeDepths**: BFS-based depth computation for phase assignment
- **DAGValidation**: Result type with validation status, topological order, depths, and errors

### API/Interface Contracts
```go
// internal/prd/merger.go (continued)

// DAGValidation holds the results of DAG validation.
type DAGValidation struct {
    Valid             bool              // true if graph is a valid DAG
    TopologicalOrder  []string          // Task IDs in topological order (empty if invalid)
    Depths            map[string]int    // Task ID -> topological depth (0 = no deps)
    MaxDepth          int               // Maximum depth in the graph
    Errors            []DAGError        // Validation errors found
}

// DAGError represents a specific validation error in the dependency graph.
type DAGError struct {
    Type    DAGErrorType
    TaskID  string   // Task with the error (or first task in cycle)
    Details string   // Human-readable description
    Cycle   []string // For CycleDetected: ordered list of task IDs forming the cycle
}

// DAGErrorType enumerates the types of DAG validation errors.
type DAGErrorType int

const (
    // DanglingReference means a task depends on a nonexistent task ID.
    DanglingReference DAGErrorType = iota
    // SelfReference means a task lists itself as a dependency.
    SelfReference
    // CycleDetected means a cycle exists in the dependency graph.
    CycleDetected
)

// ValidateDAG checks the task dependency graph for:
// 1. Dangling references (dependencies on nonexistent task IDs)
// 2. Self-references (task depending on itself)
// 3. Cycles (using Kahn's algorithm)
// If valid, also computes topological order and depth for phase assignment.
func ValidateDAG(tasks []MergedTask) *DAGValidation

// TopologicalDepths computes the depth of each task in the DAG.
// Depth 0 = no dependencies, depth N = longest path from a root.
// Requires a valid DAG (no cycles). Call after ValidateDAG confirms validity.
func TopologicalDepths(tasks []MergedTask) map[string]int
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/prd (T-060) | - | MergedTask type with Dependencies field |
| sort | stdlib | Stable ordering of nodes at same depth |

## Acceptance Criteria
- [ ] Detects dangling references (dependency on task ID not in the task list) with clear error message including both the referencing task and the missing ID
- [ ] Detects self-references (task depending on itself) with clear error message
- [ ] Detects cycles using Kahn's algorithm and reports the exact task IDs forming the cycle
- [ ] Valid DAG produces topological order and depth assignments
- [ ] Depth 0 assigned to tasks with no dependencies
- [ ] Depth N assigned based on longest path from any root (not shortest)
- [ ] MaxDepth correctly reflects the deepest task
- [ ] Multiple independent errors are all reported (not just the first)
- [ ] Topological order is deterministic (stable sort by ID for nodes at same level)
- [ ] Unit tests achieve 95% coverage

## Testing Requirements
### Unit Tests
- Linear chain A->B->C: valid DAG, depths [A:0, B:1, C:2], order [A,B,C]
- Diamond: A->B, A->C, B->D, C->D: valid, depth of D is 2 (longest path)
- Simple cycle A->B->A: detected, cycle reported as [A,B,A]
- Longer cycle A->B->C->A: detected with full cycle path
- Self-reference A->A: detected as SelfReference error
- Dangling reference A->Z where Z does not exist: detected as DanglingReference
- Multiple independent errors: self-ref + dangling ref both reported
- No dependencies at all (all depth 0): valid DAG
- Single task with no deps: valid, depth 0
- Large DAG (50+ tasks): validates correctly and computes depths
- Disconnected components (two separate chains): both validated, depths computed independently
- Multiple cycles in same graph: all detected
- Topological order is deterministic across runs (stable sort by ID)

### Integration Tests
- ValidateDAG on output of full merge pipeline (after T-060 + T-061 + T-062)
- Depth map used to generate phases.conf groupings

### Edge Cases to Handle
- Empty task list: valid DAG with no order and no depths
- Task with dependency on an empty string: treated as dangling reference
- Very deep graph (depth 100+): no stack overflow (BFS, not DFS)
- Graph with multiple roots (many tasks with no deps)
- Graph with single sink (all paths converge to one final task)
- Cycle involving only two tasks vs cycle involving many tasks

## Implementation Notes
### Recommended Approach
1. Build task ID set for referential integrity checks
2. First pass: check for self-references and dangling references; collect errors
3. Build adjacency list (task -> list of dependents) and in-degree map
4. Run Kahn's algorithm:
   a. Initialize queue with all tasks having in-degree 0
   b. Process queue: for each task, add to topological order, decrement in-degree of dependents
   c. When dependent's in-degree reaches 0, add to queue
   d. Use a sorted queue (by task ID) for deterministic output
5. After Kahn's: if len(topologicalOrder) < len(tasks), remaining tasks are in cycles
6. To report the actual cycle path: from remaining unprocessed nodes, follow dependency edges to trace a cycle
7. Compute depths via BFS from roots:
   a. Initialize depth 0 for all tasks with no dependencies
   b. For each task in topological order, set depth = max(depth of all dependencies) + 1
8. Return DAGValidation with all results

### Potential Pitfalls
- Kahn's algorithm tells you WHICH nodes are in cycles but not the exact cycle path. To report the cycle path, do a DFS from any remaining node following dependency edges until you revisit a node -- that gives the cycle
- Depth must be computed using the LONGEST path from a root, not shortest. Use `depth[task] = max(depth[dep] + 1 for dep in task.deps)` traversing in topological order
- Deterministic output requires sorting nodes at each level. When multiple tasks have in-degree 0 simultaneously, process them in sorted order by ID
- The `Dependencies` field on MergedTask after T-061 should contain only global IDs (T-NNN format). Validate this assumption

### Security Considerations
- None specific to this task (pure graph algorithm on in-memory data)
- Guard against extremely large graphs (>10000 tasks) to prevent excessive memory usage in adjacency lists

## References
- [PRD Section 5.8 - Phase 3 Gather, step 4](docs/prd/PRD-Raven.md)
- [Kahn's algorithm - Wikipedia](https://en.wikipedia.org/wiki/Topological_sorting#Kahn's_algorithm)
- [Kahn's algorithm for cycle detection](https://gaultier.github.io/blog/kahns_algorithm.html)
- [Mastering Kahn's Algorithm](https://medium.com/@anandrastogi200/mastering-kahns-algorithm-topological-sorting-with-cycle-detection-in-directed-graphs-fda80062ab99)