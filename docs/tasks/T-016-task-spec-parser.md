# T-016: Task Spec Markdown Parser

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-004 |
| Blocked By | T-004 |
| Blocks | T-017, T-019, T-020, T-026, T-030 |

## Goal
Implement a markdown parser that reads task specification files (`T-XXX-description.md`) and extracts structured data into `Task` structs. This parser is the entry point for the entire task management system -- every other task-related component depends on it to discover and understand task specs.

## Background
Per PRD Section 5.3, tasks are defined as markdown files in a configurable tasks directory. The parser must extract: task ID from the filename and `# T-XXX:` title line, task title, dependencies from the `**Dependencies:**` or `Dependencies` metadata row, priority, estimated effort, and the full spec content. The PRD specifies "Parse task specs with a simple markdown parser (regex for `**Dependencies:**` line, `# T-XXX:` title)." The `Task` struct is defined in T-004 (Central Data Types) with fields: ID, Title, Status, Phase, Dependencies, SpecFile.

## Technical Specifications
### Implementation Approach
Create `internal/task/parser.go` containing functions to parse individual task spec files and to discover all task specs in a directory. Use compiled regular expressions for extracting structured fields from markdown. The parser should be tolerant of formatting variations (extra whitespace, different heading levels) while being strict about the task ID format (`T-NNN`). Parse the metadata table to extract Priority, Effort, Dependencies, and other fields.

### Key Components
- **ParseTaskSpec**: Parses a single task markdown file content into a `Task` struct
- **ParseTaskFile**: Reads a file path and delegates to ParseTaskSpec
- **DiscoverTasks**: Scans a directory for `T-XXX-*.md` files and parses all of them
- **Compiled regexes**: Pre-compiled patterns for task ID, title, dependencies, metadata rows

### API/Interface Contracts
```go
// internal/task/parser.go
package task

import (
    "regexp"
)

// Pre-compiled regexes for parsing task spec markdown files.
var (
    // Matches "# T-001: Some Title" or "# T-001 - Some Title"
    reTitleLine = regexp.MustCompile(`^#\s+T-(\d{3}):\s*(.+)$`)

    // Matches "| Dependencies | T-001, T-003 |" in metadata table
    reMetaDeps = regexp.MustCompile(`(?i)\|\s*Dependencies\s*\|\s*([^|]+)\|`)

    // Matches "| Priority | Must Have |" in metadata table
    reMetaPriority = regexp.MustCompile(`(?i)\|\s*Priority\s*\|\s*([^|]+)\|`)

    // Matches "| Estimated Effort | Medium: 6-10hrs |"
    reMetaEffort = regexp.MustCompile(`(?i)\|\s*Estimated\s+Effort\s*\|\s*([^|]+)\|`)

    // Matches "| Blocked By | T-001, T-003 |"
    reMetaBlockedBy = regexp.MustCompile(`(?i)\|\s*Blocked\s+By\s*\|\s*([^|]+)\|`)

    // Matches "| Blocks | T-005, T-006 |"
    reMetaBlocks = regexp.MustCompile(`(?i)\|\s*Blocks\s*\|\s*([^|]+)\|`)

    // Matches task ID references like "T-001", "T-123"
    reTaskRef = regexp.MustCompile(`T-(\d{3})`)

    // Matches task spec filenames like "T-001-some-description.md"
    reTaskFilename = regexp.MustCompile(`^T-(\d{3})-[\w-]+\.md$`)
)

// ParsedTaskSpec holds all data extracted from a task spec markdown file.
type ParsedTaskSpec struct {
    ID           string   // e.g., "T-016"
    Title        string   // e.g., "Task Spec Markdown Parser"
    Priority     string   // e.g., "Must Have"
    Effort       string   // e.g., "Medium: 6-10hrs"
    Dependencies []string // e.g., ["T-004"]
    BlockedBy    []string // e.g., ["T-004"]
    Blocks       []string // e.g., ["T-017", "T-019"]
    Content      string   // Full markdown content
    SpecFile     string   // File path
}

// ParseTaskSpec parses raw markdown content of a task spec file.
// Returns a ParsedTaskSpec or an error if the file does not contain
// a valid task spec (missing title line or invalid format).
func ParseTaskSpec(content string) (*ParsedTaskSpec, error)

// ParseTaskFile reads a task spec file from disk and parses it.
func ParseTaskFile(path string) (*ParsedTaskSpec, error)

// DiscoverTasks scans a directory for T-XXX-*.md files, parses each,
// and returns a slice of ParsedTaskSpec sorted by task ID.
func DiscoverTasks(dir string) ([]*ParsedTaskSpec, error)

// ToTask converts a ParsedTaskSpec to the central Task type (from T-004).
// The phase field is populated later by cross-referencing with phases.conf.
func (p *ParsedTaskSpec) ToTask() *Task

// extractTaskRefs extracts all T-NNN references from a string.
// Used internally to parse dependency fields.
func extractTaskRefs(s string) []string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| regexp | stdlib | Compiled regex patterns for markdown parsing |
| os | stdlib | File reading |
| path/filepath | stdlib | Directory scanning with filepath.Glob |
| sort | stdlib | Sorting discovered tasks by ID |
| strings | stdlib | String manipulation for field extraction |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] ParseTaskSpec extracts task ID from `# T-XXX: Title` heading
- [ ] ParseTaskSpec extracts dependencies from metadata table `| Dependencies |` row
- [ ] ParseTaskSpec extracts priority and effort from metadata table
- [ ] ParseTaskSpec extracts blocked-by and blocks lists from metadata table
- [ ] ParseTaskSpec returns error for files without valid task ID heading
- [ ] DiscoverTasks finds all `T-XXX-*.md` files in a directory
- [ ] DiscoverTasks returns tasks sorted by ID (T-001 before T-002)
- [ ] DiscoverTasks skips non-task files (INDEX.md, PROGRESS.md)
- [ ] ToTask produces a valid Task struct matching T-004's definition
- [ ] Handles "None" as a dependency value (empty dependency list)
- [ ] Unit tests achieve 95% coverage
- [ ] All exported functions have doc comments

## Testing Requirements
### Unit Tests
- Parse a well-formed task spec: verify all fields extracted correctly
- Parse spec with no dependencies ("None"): Dependencies slice is empty
- Parse spec with multiple dependencies ("T-001, T-003, T-005"): all three extracted
- Parse spec with different heading formats ("# T-001: Title" vs "# T-001 - Title")
- Parse spec with extra whitespace in metadata table cells
- Parse spec missing title line: returns error
- Parse spec with malformed task ID: returns error
- DiscoverTasks on directory with 5 task files: returns 5 specs sorted
- DiscoverTasks on empty directory: returns empty slice, no error
- DiscoverTasks skips INDEX.md and PROGRESS.md
- extractTaskRefs("T-001, T-003") returns ["T-001", "T-003"]
- extractTaskRefs("None") returns empty slice
- Filename regex matches "T-001-setup.md" but not "T-001.md" or "README.md"

### Integration Tests
- Parse actual task files from `docs/tasks/` in the Raven project itself
- DiscoverTasks on `testdata/task-specs/` fixture directory

### Edge Cases to Handle
- Task file with Windows line endings (CRLF)
- Task file with UTF-8 BOM
- Task file with no metadata table (only title line)
- Dependencies with inconsistent spacing ("T-001,T-003" vs "T-001, T-003")
- Very large task files (>100KB)
- Task ID with leading zeros preserved ("T-001" not "T-1")
- Duplicate task IDs in the same directory (return error)

## Implementation Notes
### Recommended Approach
1. Define pre-compiled regex patterns as package-level variables (avoid recompilation per call)
2. ParseTaskSpec splits content by lines, scans for title line first
3. After finding title, scan for metadata table (rows starting with `|`)
4. For each metadata row, try matching against known field patterns
5. extractTaskRefs uses FindAllStringSubmatch to find all T-NNN patterns
6. DiscoverTasks uses `filepath.Glob(dir + "/T-[0-9][0-9][0-9]-*.md")` for discovery
7. Sort results using `sort.Slice` by task ID string (lexicographic works for zero-padded IDs)
8. Create test fixtures in `testdata/task-specs/` with representative examples

### Potential Pitfalls
- Do not use `regexp.MustCompile` inside functions -- compile once at package level
- The metadata table format may vary: some rows may have trailing spaces, inconsistent pipe alignment
- Task IDs must be zero-padded to 3 digits for correct sorting ("T-001" not "T-1")
- Dependencies field may contain "None" as a value -- treat as empty list, not as a task reference
- Be careful with multiline regex: the `^` anchor in `(?m)` mode is needed for line-start matching

### Security Considerations
- Validate file paths returned by DiscoverTasks to prevent directory traversal
- Cap file size when reading task specs to prevent memory exhaustion (reasonable limit: 1MB per file)

## References
- [PRD Section 5.3 - Task Management System](docs/prd/PRD-Raven.md)
- [Go regexp package](https://pkg.go.dev/regexp)
- [Go filepath.Glob](https://pkg.go.dev/path/filepath#Glob)
