# T-018: Phase Configuration Parser

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 3-5hrs |
| Dependencies | T-004 |
| Blocked By | T-004 |
| Blocks | T-019, T-020, T-029, T-030 |

## Goal
Implement the parser for `phases.conf`, a pipe-delimited configuration file that defines project phases with their ID, name, start task, and end task. This provides the phase-to-task mapping that the next-task selector, status command, and implementation loop all use to scope their operations to specific phases.

## Background
Per PRD Section 5.3, phase configuration is stored in `phases.conf` with the format:
```
1|Foundation & Setup|T-001|T-010
2|Core Implementation|T-011|T-020
3|Review Pipeline|T-021|T-030
```
Each line defines a phase with: `phase_id|phase_name|start_task|end_task`. The `Phase` struct is defined in T-004 with fields: ID (int), Name (string), StartTask (string), EndTask (string). The file path is configured via `project.phases_conf` in `raven.toml` (default: `docs/tasks/phases.conf`).

## Technical Specifications
### Implementation Approach
Create `internal/task/phases.go` containing functions to parse `phases.conf` into a slice of `Phase` structs. Also provide lookup methods to find which phase a given task belongs to, and to get the task ID range for a specific phase. The parser is straightforward pipe-delimited text parsing.

### Key Components
- **LoadPhases**: Reads and parses the phases.conf file
- **PhaseForTask**: Determines which phase a given task ID belongs to
- **PhaseByID**: Looks up a phase by its numeric ID
- **TaskRange**: Returns the numeric range of task IDs within a phase

### API/Interface Contracts
```go
// internal/task/phases.go
package task

import (
    "fmt"
    "strconv"
    "strings"
)

// LoadPhases reads a phases.conf file and returns a sorted slice of Phase structs.
// Returns an error if the file cannot be read or contains malformed lines.
func LoadPhases(path string) ([]Phase, error)

// ParsePhaseLine parses a single pipe-delimited line into a Phase struct.
// Expected format: "1|Foundation & Setup|T-001|T-010"
func ParsePhaseLine(line string) (*Phase, error)

// PhaseForTask returns the Phase that contains the given task ID,
// based on the task ID's numeric component falling within [StartTask, EndTask].
// Returns nil if no phase contains the task.
func PhaseForTask(phases []Phase, taskID string) *Phase

// PhaseByID returns the Phase with the given numeric ID.
// Returns nil if no phase has that ID.
func PhaseByID(phases []Phase, id int) *Phase

// TaskIDNumber extracts the numeric portion of a task ID (e.g., "T-016" -> 16).
func TaskIDNumber(taskID string) (int, error)

// TasksInPhase returns all task IDs (as strings) that fall within a phase's range.
// For example, phase with StartTask="T-001" EndTask="T-010" returns
// ["T-001", "T-002", ..., "T-010"].
func TasksInPhase(phase Phase) []string

// FormatPhaseLine formats a Phase back into pipe-delimited form.
func FormatPhaseLine(phase Phase) string

// ValidatePhases checks that phases are non-overlapping, sequential,
// and have valid task ID ranges.
func ValidatePhases(phases []Phase) error
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| os | stdlib | File reading |
| strings | stdlib | Pipe-delimited line splitting |
| strconv | stdlib | Phase ID integer parsing |
| fmt | stdlib | Error wrapping, task ID formatting |
| sort | stdlib | Sorting phases by ID |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] LoadPhases reads a valid `phases.conf` and returns correct Phase structs
- [ ] LoadPhases returns error for non-existent file
- [ ] LoadPhases skips empty lines and comment lines (starting with `#`)
- [ ] ParsePhaseLine correctly extracts ID, Name, StartTask, EndTask
- [ ] ParsePhaseLine returns error for malformed lines (wrong field count, non-numeric ID)
- [ ] PhaseForTask returns correct phase for a task within range
- [ ] PhaseForTask returns nil for a task outside all phase ranges
- [ ] PhaseByID returns correct phase for valid ID
- [ ] PhaseByID returns nil for non-existent phase ID
- [ ] TaskIDNumber correctly extracts numeric portion
- [ ] TasksInPhase generates correct list of task IDs
- [ ] ValidatePhases detects overlapping phase ranges
- [ ] ValidatePhases detects non-sequential phase IDs
- [ ] Round-trip: ParsePhaseLine(FormatPhaseLine(phase)) produces identical phase
- [ ] Unit tests achieve 95% coverage

## Testing Requirements
### Unit Tests
- ParsePhaseLine("1|Foundation & Setup|T-001|T-010"): ID=1, Name="Foundation & Setup", etc.
- ParsePhaseLine with extra whitespace around pipes: fields trimmed correctly
- ParsePhaseLine with too few fields: returns error
- ParsePhaseLine with non-numeric ID: returns error
- LoadPhases on 3-phase file: returns 3 phases sorted by ID
- LoadPhases on file with empty lines and comments: only data lines parsed
- LoadPhases on empty file: returns empty slice, no error
- PhaseForTask with "T-005" in phase 1 (T-001 to T-010): returns phase 1
- PhaseForTask with "T-015" in phase 2 (T-011 to T-020): returns phase 2
- PhaseForTask with "T-099" outside all phases: returns nil
- PhaseByID(1): returns first phase
- PhaseByID(99): returns nil
- TaskIDNumber("T-016"): returns 16
- TaskIDNumber("T-001"): returns 1
- TaskIDNumber("invalid"): returns error
- TasksInPhase for range T-001 to T-003: returns ["T-001", "T-002", "T-003"]
- ValidatePhases with overlapping ranges: returns error
- ValidatePhases with valid phases: returns nil

### Integration Tests
- Parse the actual `docs/tasks/phases.conf` in the Raven project

### Edge Cases to Handle
- Phase with single task (StartTask == EndTask)
- Phase with StartTask > EndTask (invalid, return error)
- Task ID at exact boundary between two phases (belongs to phase where EndTask matches)
- phases.conf with Windows line endings
- Very large phase ranges (100+ tasks per phase)
- Phase names with pipe characters (would break parsing -- document as invalid)

## Implementation Notes
### Recommended Approach
1. ParsePhaseLine: `strings.SplitN(line, "|", 4)`, validate field count, parse ID with `strconv.Atoi`
2. Trim whitespace from each field after splitting
3. LoadPhases: read file, split by newlines, skip empty/comment lines, parse each
4. PhaseForTask: extract numeric ID from task, check if it falls in [start, end] for each phase
5. TaskIDNumber: use regex or `strings.TrimPrefix(taskID, "T-")` then `strconv.Atoi`
6. TasksInPhase: iterate from start to end numeric IDs, format as "T-%03d"
7. ValidatePhases: check for overlaps by sorting and verifying end[i] < start[i+1]
8. Create test fixture at `testdata/phases.conf`

### Potential Pitfalls
- Task IDs must be zero-padded to 3 digits ("T-001" not "T-1") for consistent formatting
- Phase ID 0 is technically valid but unusual -- decide whether to allow or reject
- When checking if a task is in a phase, compare numeric values not strings ("T-010" < "T-009" lexicographically)
- LoadPhases should sort by phase ID even if the file is unordered

### Security Considerations
- Validate file path before reading to prevent directory traversal
- Cap file size (phases.conf should never be large)

## References
- [PRD Section 5.3 - Task Management System](docs/prd/PRD-Raven.md)
- [PRD Section 6.4 - Phase struct definition](docs/prd/PRD-Raven.md)
