# T-064: Task File Emitter

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Large: 16-24hrs |
| Dependencies | T-056, T-060, T-061, T-062, T-063 |
| Blocked By | T-062, T-063 |
| Blocks | T-065 |

## Goal
Implement the final step of the merge phase (Phase 3, steps 5-6): auto-generate `phases.conf` from topological depth, and emit all output files from the merged, deduplicated, DAG-validated task list. Output files include: individual `T-XXX-slug.md` task specification files, `task-state.conf` (all tasks as `not_started`), `phases.conf` (phase groupings from topological depth), `PROGRESS.md` (initial progress tracking file), and `INDEX.md` (task index with summary tables and dependency graph). All file generation uses Go `text/template` for consistent, maintainable formatting.

## Background
Per PRD Section 5.8, Phase 3 steps 5-6 are: "Auto-generates `phases.conf` from topological depth" and "Emits: `T-XXX-description.md` files, `task-state.conf`, `phases.conf`, `PROGRESS.md`, `INDEX.md`." This is the culmination of the entire PRD decomposition pipeline -- it transforms the in-memory merged task structures into the on-disk file layout that Raven's task management system (internal/task) consumes.

The file formats are defined throughout the PRD:
- `task-state.conf`: pipe-delimited with columns `task_id | status | agent | timestamp | notes` (Section 5.3)
- `phases.conf`: pipe-delimited with columns `phase_id|phase_name|start_task|end_task` (Section 5.3)
- `T-XXX-slug.md`: markdown task spec with title, metadata table, goal, dependencies, acceptance criteria (Section 5.3)
- `PROGRESS.md`: markdown progress log updated after each task completion (Section 5.3)
- `INDEX.md`: markdown task index with phase tables, dependency overview, and summary statistics

## Technical Specifications
### Implementation Approach
Create `internal/prd/emitter.go` containing an `Emitter` struct that takes the final `[]MergedTask`, the `DAGValidation` result (for depths and topological order), and an output directory. It generates all output files using `text/template` templates. Phase assignment uses the topological depth map: tasks at the same depth are grouped into the same phase. Phase names are auto-generated from the predominant epic title at each depth level, or a generic "Phase N" label.

### Key Components
- **Emitter**: Orchestrates all file generation
- **EmitResult**: Summary of generated files with paths and counts
- **TaskTemplate**: Template for individual `T-XXX-slug.md` files
- **PhaseAssigner**: Groups tasks by topological depth into phases
- **SlugGenerator**: Converts task titles to kebab-case file slugs

### API/Interface Contracts
```go
// internal/prd/emitter.go

type Emitter struct {
    outputDir string
    logger    *log.Logger
}

type EmitOpts struct {
    Tasks      []MergedTask
    Validation *DAGValidation    // From T-063, provides depths and topo order
    Epics      *EpicBreakdown    // For phase naming context
    StartID    int               // Starting task number (default 1)
}

type EmitResult struct {
    OutputDir       string
    TaskFiles       []string  // Paths to generated T-XXX-slug.md files
    TaskStateFile   string    // Path to task-state.conf
    PhasesFile      string    // Path to phases.conf
    ProgressFile    string    // Path to PROGRESS.md
    IndexFile       string    // Path to INDEX.md
    TotalTasks      int
    TotalPhases     int
}

type PhaseInfo struct {
    ID        int
    Name      string
    StartTask string  // e.g., T-001
    EndTask   string  // e.g., T-010
    Tasks     []MergedTask
}

func NewEmitter(outputDir string, opts ...EmitterOption) *Emitter

// Emit generates all output files from the merged task data.
func (e *Emitter) Emit(opts EmitOpts) (*EmitResult, error)

// AssignPhases groups tasks by topological depth into numbered phases.
func AssignPhases(tasks []MergedTask, depths map[string]int, epics *EpicBreakdown) []PhaseInfo

// Slugify converts a task title to a kebab-case slug suitable for filenames.
// e.g., "Set Up Authentication Middleware" -> "set-up-authentication-middleware"
func Slugify(title string) string

// ResequenceIDs re-assigns sequential IDs (T-001, T-002, ...) to close gaps
// left by deduplication. Updates all internal dependency references.
func ResequenceIDs(tasks []MergedTask) ([]MergedTask, IDMapping)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/prd (T-056, T-060, T-063) | - | MergedTask, DAGValidation, EpicBreakdown types |
| text/template | stdlib | File content generation |
| os | stdlib | File and directory creation |
| path/filepath | stdlib | Cross-platform path handling |
| fmt | stdlib | ID formatting |
| strings | stdlib | Slug generation |
| regexp | stdlib | Slug sanitization |
| time | stdlib | Timestamp in task-state.conf |
| unicode | stdlib | Character classification for slugify |

## Acceptance Criteria
- [ ] Generates individual `T-XXX-slug.md` files for each task with: title, metadata table (priority, effort, dependencies), goal/description, acceptance criteria
- [ ] Generates `task-state.conf` with one line per task: `task_id | not_started | | | ` (pipe-delimited, empty agent/timestamp/notes)
- [ ] Generates `phases.conf` with one line per phase: `phase_id|Phase Name|T-start|T-end` (pipe-delimited)
- [ ] Phases are derived from topological depth: depth 0 = Phase 1, depth 1 = Phase 2, etc.
- [ ] Generates `PROGRESS.md` with initial structure (header, empty completion log, summary statistics)
- [ ] Generates `INDEX.md` with task summary table, phase groupings, and dependency overview
- [ ] Task file slugs are valid filenames: lowercase, kebab-case, no special characters, max 50 chars
- [ ] Global IDs are re-sequenced to close gaps from deduplication (T-001, T-002, ... with no gaps)
- [ ] All dependency references in generated files use final re-sequenced IDs
- [ ] Output directory is created if it does not exist
- [ ] Existing files in output directory are NOT overwritten unless --force is used (safety)
- [ ] EmitResult accurately lists all generated file paths
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- Slugify("Set Up Authentication Middleware") returns "set-up-authentication-middleware"
- Slugify("Create CI/CD Pipeline") returns "create-ci-cd-pipeline"
- Slugify("   Extra   Spaces   ") returns "extra-spaces"
- Slugify with very long title truncates to 50 chars at word boundary
- Slugify with unicode characters strips non-ASCII
- AssignPhases with depths {A:0, B:0, C:1, D:2} produces 3 phases
- AssignPhases with all tasks at depth 0 produces 1 phase
- ResequenceIDs closes gaps: [T-001, T-003, T-005] becomes [T-001, T-002, T-003]
- ResequenceIDs updates dependency references to new IDs
- Emit generates correct number of task files
- Generated task-state.conf has correct pipe-delimited format
- Generated phases.conf has correct pipe-delimited format with start/end task IDs
- Generated T-XXX-slug.md contains all required sections
- Generated INDEX.md contains task summary table
- Generated PROGRESS.md contains initial header and structure
- Output directory created when it does not exist
- Error returned when output directory exists and has task files (without --force)

### Integration Tests
- Full emit from realistic merged task set (15+ tasks, 3-4 phases)
- Generated files parse correctly with internal/task parsers (task spec parser, phases parser, state parser)
- Round-trip: emit files -> read back with task parsers -> verify data matches

### Edge Cases to Handle
- Task title that produces an empty slug after sanitization (use task ID as fallback slug)
- Two tasks producing identical slugs (append numeric suffix: -2, -3)
- Task with no acceptance criteria (generate section with "TBD" placeholder)
- Task with no dependencies (Dependencies line shows "None")
- Very large task set (500+ tasks) -- ensure file generation is performant
- Output directory on read-only filesystem (return clear error)
- Task descriptions containing template syntax ({{ }}) -- must be escaped in template output

## Implementation Notes
### Recommended Approach
1. **ResequenceIDs** first: close gaps from dedup, build old->new ID mapping, update all deps
2. **AssignPhases**: group tasks by depth, assign phase numbers, generate phase names
3. **Generate task files**: for each task, render T-XXX-slug.md template and write to outputDir
4. **Generate task-state.conf**: one line per task in phase/ID order
5. **Generate phases.conf**: one line per phase with start/end task IDs
6. **Generate PROGRESS.md**: header, total tasks, phase summary, empty completion log
7. **Generate INDEX.md**: task table with ID/title/priority/effort/deps/phase, dependency graph
8. Return EmitResult with all generated file paths

Template structure for T-XXX-slug.md:
```
# {{.GlobalID}}: {{.Title}}

## Metadata
| Field | Value |
|-------|-------|
| Priority | {{.Priority}} |
| Estimated Effort | {{.Effort}} |
| Dependencies | {{.DependenciesStr}} |

## Goal
{{.Description}}

## Acceptance Criteria
{{range .AcceptanceCriteria}}- [ ] {{.}}
{{end}}
```

### Potential Pitfalls
- Template escaping: if task descriptions contain `{{` or `}}`, Go's text/template will try to interpret them. Use a custom delimiter (e.g., `<<` `>>`) or pre-escape the content
- Slug collision: two tasks may have titles that slugify identically. Detect and append suffixes before writing files
- Phase start/end task IDs must reflect the re-sequenced IDs, not the original IDs
- File permissions: create files with 0644, directories with 0755
- The `Dependencies` field in task files should use the Raven convention: `**Dependencies:** T-001, T-003` or `**Dependencies:** None`
- INDEX.md should include a Mermaid dependency graph if the task count is reasonable (<100 tasks)

### Security Considerations
- Validate output directory path to prevent path traversal
- Sanitize task titles thoroughly before using as filenames
- Do not write files outside the designated output directory
- Cap total output size to prevent disk exhaustion (warn if >1000 tasks)

## References
- [PRD Section 5.8 - Phase 3 Gather, steps 5-6](docs/prd/PRD-Raven.md)
- [PRD Section 5.3 - Task state management, phases.conf format](docs/prd/PRD-Raven.md)
- [Go text/template documentation](https://pkg.go.dev/text/template)
- [Go os.MkdirAll documentation](https://pkg.go.dev/os#MkdirAll)
- [Go filepath package](https://pkg.go.dev/path/filepath)