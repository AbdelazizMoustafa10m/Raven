# T-017: Task State Management

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 8-12hrs |
| Dependencies | T-004, T-016 |
| Blocked By | T-004, T-016 |
| Blocks | T-019, T-020, T-027, T-029, T-030 |

## Goal
Implement the task state management system that reads and writes `task-state.conf`, a pipe-delimited file tracking the status of every task. This is the persistent state layer for the task system -- the implementation loop, status command, and next-task selector all depend on it to know which tasks are completed, in progress, or blocked.

## Background
Per PRD Section 5.3, task state is tracked in a pipe-delimited state file (`task-state.conf`) with columns: `task_id | status | agent | timestamp | notes`. Supported task statuses are: `not_started`, `in_progress`, `completed`, `blocked`, `skipped`. The PRD specifies "Task state file uses file locking (`flock`) for concurrent access safety." The file path is configured via `project.task_state_file` in `raven.toml` (default: `docs/tasks/task-state.conf`).

Example `task-state.conf`:
```
T-001|completed|claude|2026-02-17T10:30:00Z|Foundation setup done
T-002|completed|claude|2026-02-17T11:45:00Z|
T-003|in_progress|codex|2026-02-17T12:00:00Z|Working on config
T-004|not_started|||
```

## Technical Specifications
### Implementation Approach
Create `internal/task/state.go` containing a `StateManager` that reads, writes, and updates the task state file. Use file-level locking for concurrent access safety (important when multiple Raven processes or the TUI and a workflow run simultaneously). The state file format is simple pipe-delimited text with one line per task. Provide atomic updates via a read-modify-write pattern with file locking.

### Key Components
- **TaskStatus**: Typed string constants for task statuses
- **TaskState**: Represents a single task's state entry (one line in the file)
- **StateManager**: Reads, writes, and queries the state file with file locking
- **File locking**: Uses `syscall.Flock` on Unix, or write-to-temp-then-rename pattern for cross-platform safety

### API/Interface Contracts
```go
// internal/task/state.go
package task

import (
    "time"
)

// TaskStatus represents the status of a task in the state file.
type TaskStatus string

const (
    StatusNotStarted TaskStatus = "not_started"
    StatusInProgress TaskStatus = "in_progress"
    StatusCompleted  TaskStatus = "completed"
    StatusBlocked    TaskStatus = "blocked"
    StatusSkipped    TaskStatus = "skipped"
)

// ValidStatuses returns all valid task status values.
func ValidStatuses() []TaskStatus

// IsValid returns true if the status is a recognized value.
func (s TaskStatus) IsValid() bool

// TaskState represents a single row in task-state.conf.
type TaskState struct {
    TaskID    string     `json:"task_id"`
    Status    TaskStatus `json:"status"`
    Agent     string     `json:"agent"`
    Timestamp time.Time  `json:"timestamp"`
    Notes     string     `json:"notes"`
}

// StateManager manages the task-state.conf file.
type StateManager struct {
    filePath string
}

// NewStateManager creates a StateManager for the given state file path.
func NewStateManager(filePath string) *StateManager

// Load reads the state file and returns all task states.
// If the file does not exist, returns an empty slice (not an error).
func (sm *StateManager) Load() ([]TaskState, error)

// LoadMap reads the state file and returns a map of task_id -> TaskState.
func (sm *StateManager) LoadMap() (map[string]*TaskState, error)

// Get returns the state for a specific task ID.
// Returns nil if the task has no state entry (implicitly not_started).
func (sm *StateManager) Get(taskID string) (*TaskState, error)

// Update sets the state for a specific task. If the task does not exist
// in the file, a new line is appended. If it exists, the line is updated.
// Uses file locking for concurrent safety.
func (sm *StateManager) Update(state TaskState) error

// UpdateStatus is a convenience method that updates only the status,
// agent, and timestamp for a task, preserving existing notes.
func (sm *StateManager) UpdateStatus(taskID string, status TaskStatus, agent string) error

// Initialize creates a state file with not_started entries for all
// provided task IDs. Does not overwrite existing entries.
func (sm *StateManager) Initialize(taskIDs []string) error

// StatusCounts returns a map of status -> count for all tasks.
func (sm *StateManager) StatusCounts() (map[TaskStatus]int, error)

// TasksWithStatus returns all task IDs with the given status.
func (sm *StateManager) TasksWithStatus(status TaskStatus) ([]string, error)

// parseLine parses a single pipe-delimited line into a TaskState.
func parseLine(line string) (*TaskState, error)

// formatLine formats a TaskState as a pipe-delimited line.
func formatLine(state TaskState) string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| os | stdlib | File I/O |
| strings | stdlib | Pipe-delimited parsing |
| time | stdlib | Timestamp formatting (RFC3339) |
| syscall | stdlib | File locking (Unix) |
| fmt | stdlib | Error wrapping |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Load reads a well-formed `task-state.conf` and returns all entries
- [ ] Load returns empty slice (not error) when file does not exist
- [ ] LoadMap returns a map keyed by task ID
- [ ] Get returns the correct TaskState for an existing task
- [ ] Get returns nil for a task not in the state file
- [ ] Update appends new tasks and modifies existing ones
- [ ] Update uses file locking to prevent concurrent corruption
- [ ] UpdateStatus convenience method works correctly
- [ ] Initialize creates entries for all provided task IDs without overwriting existing entries
- [ ] StatusCounts returns correct counts per status
- [ ] TasksWithStatus filters correctly
- [ ] Pipe-delimited format round-trips correctly (parse then format produces identical line)
- [ ] Timestamps are formatted as RFC3339
- [ ] All five TaskStatus constants are valid
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- parseLine for a complete line: all fields extracted correctly
- parseLine for a minimal line (empty agent, timestamp, notes): fields default correctly
- parseLine for a malformed line (wrong number of fields): returns error
- formatLine produces correct pipe-delimited output
- Round-trip: parseLine(formatLine(state)) produces identical state
- Load on a file with 5 entries: returns 5 TaskStates
- Load on non-existent file: returns empty slice, nil error
- LoadMap produces correct key-value pairs
- Get for existing task: returns correct state
- Get for non-existent task: returns nil
- Update adds new task to file
- Update modifies existing task in file
- UpdateStatus sets status, agent, and timestamp correctly
- Initialize creates entries without overwriting existing ones
- StatusCounts with mixed statuses: correct counts
- TasksWithStatus filters for completed: returns only completed tasks
- IsValid returns true for all five status constants
- IsValid returns false for "unknown_status"

### Integration Tests
- Concurrent Updates: two goroutines updating different tasks simultaneously (verifies file locking)
- Large state file: 100+ task entries, load and update performance

### Edge Cases to Handle
- Empty state file (0 bytes): treated as no entries
- State file with trailing newline: does not create phantom entry
- State file with blank lines: skipped gracefully
- Notes field containing pipe characters: only split on first 4 pipes (notes is everything after)
- Timestamp in non-RFC3339 format: attempt best-effort parse or record original string
- Task ID with different padding (T-01 vs T-001): preserve as-is
- File permissions: state file not writable (return clear error)
- Concurrent writes from two processes (file locking must prevent corruption)

## Implementation Notes
### Recommended Approach
1. Define TaskStatus constants and validation method first
2. Implement parseLine/formatLine as the foundation
3. Use `strings.SplitN(line, "|", 5)` to split -- limit to 5 fields so notes can contain pipes
4. Timestamps: use `time.RFC3339` format for parsing and formatting
5. For file locking on Unix: open file, call `syscall.Flock(fd, syscall.LOCK_EX)`, perform read-modify-write, unlock
6. For cross-platform simplicity: write to a temp file in the same directory, then `os.Rename` (atomic on Unix)
7. The atomic write pattern is simpler and more portable than flock -- prefer it for v2.0
8. Initialize: load existing entries, merge with new task IDs (preserving existing), write back
9. Use `t.TempDir()` in tests to avoid polluting the workspace
10. Create fixture files in `testdata/` for known-good state files

### Potential Pitfalls
- The notes field may contain pipe characters -- use `SplitN` with limit 5, not `Split`
- File locking with `syscall.Flock` is not available on Windows -- use the atomic write pattern as a portable alternative
- When updating an existing entry, read the entire file, modify the target line, write the entire file back (no in-place editing of text files)
- Empty timestamp and agent fields still need pipe delimiters in the output
- Be careful with trailing newlines when writing: ensure exactly one newline at end of file

### Security Considerations
- Validate task IDs before writing to prevent injection of malformed lines
- State file should be written with restrictive permissions (0644)
- File locking prevents corruption but does not prevent malicious concurrent access -- acceptable for CLI tool

## References
- [PRD Section 5.3 - Task Management System](docs/prd/PRD-Raven.md)
- [Cross Platform File Locking with Go](https://www.chronohq.com/blog/cross-platform-file-locking-with-go)
- [gofrs/flock - Thread-safe file locking](https://github.com/gofrs/flock)
- [Go time.RFC3339 format](https://pkg.go.dev/time#pkg-constants)
