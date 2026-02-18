package task

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// maxPhasesFileSize is the maximum number of bytes read from phases.conf.
// Phase files are always small; this guards against accidental huge reads.
const maxPhasesFileSize = 64 * 1024 // 64 KiB

// LoadPhases reads and parses a phases.conf file, returning a slice of Phase
// structs sorted by ascending phase ID.
//
// The file format is pipe-delimited with the following fields:
//
//	ID|SLUG|DISPLAY_NAME|TASK_START|TASK_END|ICON
//	 1|foundation|Foundation|001|015|🏗
//
// Empty lines and lines whose first non-space character is '#' are skipped.
// Returns an error if the file cannot be read or contains malformed lines.
func LoadPhases(path string) ([]Phase, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("loading phases file %q: %w", path, err)
	}
	if info.Size() > maxPhasesFileSize {
		return nil, fmt.Errorf("loading phases file %q: file exceeds 64 KiB limit", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("loading phases file %q: %w", path, err)
	}
	defer f.Close() //nolint:errcheck

	var phases []Phase
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Normalise Windows line endings just in case.
		line = strings.TrimRight(line, "\r")

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		p, err := ParsePhaseLine(trimmed)
		if err != nil {
			return nil, fmt.Errorf("loading phases file %q line %d: %w", path, lineNum, err)
		}
		phases = append(phases, *p)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning phases file %q: %w", path, err)
	}

	// Sort by phase ID so callers always receive a deterministic order
	// regardless of file order.
	sort.Slice(phases, func(i, j int) bool {
		return phases[i].ID < phases[j].ID
	})

	return phases, nil
}

// ParsePhaseLine parses a single pipe-delimited phases.conf line into a Phase
// struct.
//
// Two formats are accepted:
//
//  1. Four-field: "1|Foundation & Setup|T-001|T-010"
//     (phase_id|name|start_task|end_task)
//     Detected when field[2] starts with "T-".
//
//  2. Six-or-more-field: "1|foundation|Foundation|001|015|🏗"
//     (phase_id|slug|display_name|task_start_num|task_end_num|icon…)
//     Detected when field[2] does NOT start with "T-".
//
// In the six-field format the numeric task start/end are zero-padded numbers
// that are converted to canonical "T-NNN" IDs automatically.
// Leading/trailing whitespace around each field is trimmed.
// Returns an error if the line has fewer than 4 fields or a non-numeric ID.
func ParsePhaseLine(line string) (*Phase, error) {
	// Trim BOM just in case the line is the very first line of a BOM-prefixed file.
	line = strings.TrimPrefix(line, "\xef\xbb\xbf")

	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	if len(parts) < 4 {
		return nil, fmt.Errorf("parsing phase line: expected at least 4 pipe-delimited fields, got %d in %q", len(parts), line)
	}

	// Field 0: numeric phase ID.
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("parsing phase line: non-numeric phase ID %q in %q", parts[0], line)
	}

	p := &Phase{ID: id}

	// Detect format by checking whether the third field (index 2) is a task ID
	// ("T-NNN") or a display name (which means we have the six-field format
	// with ID|Slug|DisplayName|StartNum|EndNum|Icon…).
	//
	// Four-field format:  ID | Name        | T-NNN | T-NNN
	// Six-field format:   ID | slug        | Name  | NNN   | NNN | Icon
	//
	// The heuristic: if field[2] starts with "T-" it is the start task (four-
	// field); otherwise it is the display name (six-field).
	fourField := strings.HasPrefix(parts[2], "T-")

	if fourField {
		// Four-field format: ID|Name|StartTask|EndTask
		p.Name = parts[1]
		p.StartTask = parts[2]
		p.EndTask = parts[3]
	} else {
		// Six-or-more-field format: ID|Slug|DisplayName|StartNum|EndNum|Icon…
		// Require at least six fields.
		if len(parts) < 5 {
			return nil, fmt.Errorf("parsing phase line: six-field format requires at least 5 fields, got %d in %q", len(parts), line)
		}
		p.Name = parts[2]

		start, err := strconv.Atoi(parts[3])
		if err != nil {
			return nil, fmt.Errorf("parsing phase line: non-numeric task start %q in %q", parts[3], line)
		}
		end, err := strconv.Atoi(parts[4])
		if err != nil {
			return nil, fmt.Errorf("parsing phase line: non-numeric task end %q in %q", parts[4], line)
		}
		p.StartTask = fmt.Sprintf("T-%03d", start)
		p.EndTask = fmt.Sprintf("T-%03d", end)
	}

	if p.Name == "" {
		return nil, fmt.Errorf("parsing phase line: phase name is empty in %q", line)
	}
	if p.StartTask == "" || p.EndTask == "" {
		return nil, fmt.Errorf("parsing phase line: start or end task is empty in %q", line)
	}

	return p, nil
}

// PhaseForTask returns the Phase that contains the given task ID based on the
// task ID's numeric component falling within [StartTask, EndTask] (inclusive).
// Returns nil if no phase contains the task.
func PhaseForTask(phases []Phase, taskID string) *Phase {
	num, err := TaskIDNumber(taskID)
	if err != nil {
		return nil
	}

	for i := range phases {
		p := &phases[i]
		start, err := TaskIDNumber(p.StartTask)
		if err != nil {
			continue
		}
		end, err := TaskIDNumber(p.EndTask)
		if err != nil {
			continue
		}
		if num >= start && num <= end {
			return p
		}
	}
	return nil
}

// PhaseByID returns the Phase with the given numeric ID.
// Returns nil if no phase has that ID.
func PhaseByID(phases []Phase, id int) *Phase {
	for i := range phases {
		if phases[i].ID == id {
			return &phases[i]
		}
	}
	return nil
}

// TaskIDNumber extracts the numeric portion of a task ID.
// Examples: "T-016" -> 16, "T-001" -> 1.
// Returns an error if the ID does not follow the "T-NNN" pattern.
func TaskIDNumber(taskID string) (int, error) {
	trimmed := strings.TrimSpace(taskID)
	if !strings.HasPrefix(trimmed, "T-") {
		return 0, fmt.Errorf("task ID %q does not have the required 'T-' prefix", taskID)
	}
	numStr := trimmed[2:]
	if numStr == "" {
		return 0, fmt.Errorf("task ID %q has no numeric suffix", taskID)
	}
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("task ID %q has non-numeric suffix %q", taskID, numStr)
	}
	return n, nil
}

// TasksInPhase returns all task IDs (as "T-NNN" strings) that fall within a
// phase's [StartTask, EndTask] range (inclusive). Task IDs are zero-padded to
// three digits.
//
// Returns an empty slice if StartTask or EndTask cannot be parsed, or if
// StartTask > EndTask.
func TasksInPhase(phase Phase) []string {
	start, err := TaskIDNumber(phase.StartTask)
	if err != nil {
		return []string{}
	}
	end, err := TaskIDNumber(phase.EndTask)
	if err != nil {
		return []string{}
	}
	if start > end {
		return []string{}
	}

	ids := make([]string, 0, end-start+1)
	for i := start; i <= end; i++ {
		ids = append(ids, fmt.Sprintf("T-%03d", i))
	}
	return ids
}

// FormatPhaseLine formats a Phase back into the canonical four-field
// pipe-delimited form: "ID|Name|StartTask|EndTask".
func FormatPhaseLine(phase Phase) string {
	return strings.Join([]string{
		strconv.Itoa(phase.ID),
		phase.Name,
		phase.StartTask,
		phase.EndTask,
	}, "|")
}

// ValidatePhases checks that the provided phases satisfy the following
// invariants:
//
//   - Every phase has a non-empty Name and non-zero ID.
//   - No two phases share the same ID.
//   - Every phase has StartTask <= EndTask (by numeric value).
//   - No two phases have overlapping task-ID ranges.
//
// Returns a non-nil error describing the first violation found.
func ValidatePhases(phases []Phase) error {
	// Check IDs for uniqueness.
	seenIDs := make(map[int]bool, len(phases))
	for _, p := range phases {
		if p.ID == 0 {
			return fmt.Errorf("validating phases: phase has ID 0 (must be positive)")
		}
		if p.Name == "" {
			return fmt.Errorf("validating phases: phase %d has empty name", p.ID)
		}
		if seenIDs[p.ID] {
			return fmt.Errorf("validating phases: duplicate phase ID %d", p.ID)
		}
		seenIDs[p.ID] = true

		start, err := TaskIDNumber(p.StartTask)
		if err != nil {
			return fmt.Errorf("validating phases: phase %d has invalid StartTask: %w", p.ID, err)
		}
		end, err := TaskIDNumber(p.EndTask)
		if err != nil {
			return fmt.Errorf("validating phases: phase %d has invalid EndTask: %w", p.ID, err)
		}
		if start > end {
			return fmt.Errorf("validating phases: phase %d StartTask %s is after EndTask %s",
				p.ID, p.StartTask, p.EndTask)
		}
	}

	// Build a copy sorted by StartTask number to check for overlaps.
	sorted := make([]Phase, len(phases))
	copy(sorted, phases)
	sort.Slice(sorted, func(i, j int) bool {
		si, _ := TaskIDNumber(sorted[i].StartTask)
		sj, _ := TaskIDNumber(sorted[j].StartTask)
		return si < sj
	})

	for i := 1; i < len(sorted); i++ {
		prevEnd, _ := TaskIDNumber(sorted[i-1].EndTask)
		curStart, _ := TaskIDNumber(sorted[i].StartTask)
		if curStart <= prevEnd {
			return fmt.Errorf(
				"validating phases: phase %d (%s-%s) overlaps with phase %d (%s-%s)",
				sorted[i-1].ID, sorted[i-1].StartTask, sorted[i-1].EndTask,
				sorted[i].ID, sorted[i].StartTask, sorted[i].EndTask,
			)
		}
	}

	return nil
}
