package task

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TaskState represents a single row in task-state.conf.
// The file format is pipe-delimited with five fields:
//
//	task_id|status|agent|timestamp|notes
type TaskState struct {
	TaskID    string     `json:"task_id"`
	Status    TaskStatus `json:"status"`
	Agent     string     `json:"agent"`
	Timestamp time.Time  `json:"timestamp"`
	Notes     string     `json:"notes"`
}

// StateManager manages the task-state.conf file. It reads, writes, and
// queries task state using an atomic write pattern (write to temp file then
// rename) for cross-platform concurrent safety. A mutex serializes concurrent
// reads and writes within the same process.
type StateManager struct {
	mu       sync.Mutex
	filePath string
}

// NewStateManager creates a StateManager for the given state file path.
func NewStateManager(filePath string) *StateManager {
	return &StateManager{filePath: filePath}
}

// Load reads the state file and returns all task states.
// If the file does not exist or is empty, returns an empty slice (not an error).
func (sm *StateManager) Load() ([]TaskState, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.load()
}

// load is the internal, mutex-free version of Load. Callers must hold sm.mu.
func (sm *StateManager) load() ([]TaskState, error) {
	f, err := os.Open(sm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []TaskState{}, nil
		}
		return nil, fmt.Errorf("loading state file %q: %w", sm.filePath, err)
	}
	defer f.Close() //nolint:errcheck

	return parseStateFile(f)
}

// LoadMap reads the state file and returns a map of task_id -> *TaskState.
// If the file does not exist, returns an empty map (not an error).
func (sm *StateManager) LoadMap() (map[string]*TaskState, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	states, err := sm.load()
	if err != nil {
		return nil, err
	}

	m := make(map[string]*TaskState, len(states))
	for i := range states {
		s := states[i]
		m[s.TaskID] = &s
	}
	return m, nil
}

// Get returns the state for a specific task ID.
// Returns nil if the task has no state entry (implicitly not_started).
func (sm *StateManager) Get(taskID string) (*TaskState, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	states, err := sm.load()
	if err != nil {
		return nil, fmt.Errorf("getting state for task %q: %w", taskID, err)
	}
	for i := range states {
		if states[i].TaskID == taskID {
			return &states[i], nil
		}
	}
	return nil, nil
}

// Update sets the state for a specific task. If the task does not exist in
// the file, a new line is appended. If it exists, the line is updated in
// place. The entire read-modify-write cycle is serialized by the internal
// mutex, and the write uses an atomic rename for cross-platform safety.
func (sm *StateManager) Update(state TaskState) error {
	if state.TaskID == "" {
		return fmt.Errorf("updating state: task ID must not be empty")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	states, err := sm.load()
	if err != nil {
		return fmt.Errorf("updating state: %w", err)
	}

	updated := false
	for i, s := range states {
		if s.TaskID == state.TaskID {
			states[i] = state
			updated = true
			break
		}
	}
	if !updated {
		states = append(states, state)
	}

	return sm.writeAtomic(states)
}

// UpdateStatus is a convenience method that updates only the status, agent,
// and timestamp for a task, preserving any existing notes. If the task has
// no existing entry, a new one is created with empty notes. The entire
// read-modify-write cycle is serialized by the internal mutex.
func (sm *StateManager) UpdateStatus(taskID string, status TaskStatus, agent string) error {
	if taskID == "" {
		return fmt.Errorf("updating status: task ID must not be empty")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	states, err := sm.load()
	if err != nil {
		return fmt.Errorf("updating status for task %q: %w", taskID, err)
	}

	// Find existing notes; build the updated entry.
	notes := ""
	updated := false
	newEntry := TaskState{
		TaskID:    taskID,
		Status:    status,
		Agent:     agent,
		Timestamp: time.Now().UTC(),
	}
	for i, s := range states {
		if s.TaskID == taskID {
			newEntry.Notes = s.Notes
			states[i] = newEntry
			updated = true
			break
		}
	}
	if !updated {
		newEntry.Notes = notes
		states = append(states, newEntry)
	}

	return sm.writeAtomic(states)
}

// Initialize creates state file entries with StatusNotStarted for all
// provided task IDs. Existing entries are preserved and not overwritten.
// The resulting file is written atomically.
func (sm *StateManager) Initialize(taskIDs []string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	ordered, err := sm.load()
	if err != nil {
		return fmt.Errorf("initializing state: %w", err)
	}

	seenIDs := make(map[string]bool, len(ordered))
	for _, s := range ordered {
		seenIDs[s.TaskID] = true
	}

	for _, id := range taskIDs {
		if id == "" {
			continue
		}
		if !seenIDs[id] {
			ordered = append(ordered, TaskState{
				TaskID: id,
				Status: StatusNotStarted,
			})
			seenIDs[id] = true
		}
	}

	return sm.writeAtomic(ordered)
}

// StatusCounts returns a map of status -> count across all task state entries.
func (sm *StateManager) StatusCounts() (map[TaskStatus]int, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	states, err := sm.load()
	if err != nil {
		return nil, fmt.Errorf("counting statuses: %w", err)
	}

	counts := make(map[TaskStatus]int)
	for _, s := range states {
		counts[s.Status]++
	}
	return counts, nil
}

// TasksWithStatus returns all task IDs whose status matches the given value.
func (sm *StateManager) TasksWithStatus(status TaskStatus) ([]string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	states, err := sm.load()
	if err != nil {
		return nil, fmt.Errorf("filtering tasks by status %q: %w", status, err)
	}

	var ids []string
	for _, s := range states {
		if s.Status == status {
			ids = append(ids, s.TaskID)
		}
	}
	return ids, nil
}

// parseStateFile reads all task state lines from f, skipping blank lines and
// comment lines (lines whose first non-space character is '#').
func parseStateFile(f *os.File) ([]TaskState, error) {
	var states []TaskState
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip blank lines and comments.
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		state, err := parseLine(trimmed)
		if err != nil {
			return nil, fmt.Errorf("parsing state file line %d: %w", lineNum, err)
		}
		states = append(states, *state)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning state file: %w", err)
	}
	return states, nil
}

// parseLine parses a single pipe-delimited state file line into a TaskState.
// The format is: task_id|status|agent|timestamp|notes
// The split is limited to 5 parts so that notes may contain pipe characters.
func parseLine(line string) (*TaskState, error) {
	parts := strings.SplitN(line, "|", 5)
	if len(parts) < 1 || strings.TrimSpace(parts[0]) == "" {
		return nil, fmt.Errorf("invalid state line: task ID is empty in %q", line)
	}

	state := &TaskState{
		TaskID: strings.TrimSpace(parts[0]),
	}

	// Status field (index 1).
	if len(parts) > 1 {
		state.Status = TaskStatus(strings.TrimSpace(parts[1]))
	}

	// Agent field (index 2).
	if len(parts) > 2 {
		state.Agent = strings.TrimSpace(parts[2])
	}

	// Timestamp field (index 3) -- best-effort RFC3339 parse; zero time on failure.
	if len(parts) > 3 {
		ts := strings.TrimSpace(parts[3])
		if ts != "" {
			t, err := time.Parse(time.RFC3339, ts)
			if err == nil {
				state.Timestamp = t
			}
			// On parse failure, Timestamp remains zero value (best-effort).
		}
	}

	// Notes field (index 4) -- may be absent or empty.
	if len(parts) > 4 {
		state.Notes = parts[4]
	}

	return state, nil
}

// formatLine formats a TaskState as a pipe-delimited state file line.
// Empty timestamp is rendered as an empty field; notes are kept verbatim.
func formatLine(state TaskState) string {
	ts := ""
	if !state.Timestamp.IsZero() {
		ts = state.Timestamp.UTC().Format(time.RFC3339)
	}
	return strings.Join([]string{
		state.TaskID,
		string(state.Status),
		state.Agent,
		ts,
		state.Notes,
	}, "|")
}

// writeAtomic writes states to a temporary file in the same directory as
// sm.filePath, then renames it atomically to sm.filePath. File permissions
// are 0644.
func (sm *StateManager) writeAtomic(states []TaskState) error {
	// Ensure the parent directory exists.
	dir := filepath.Dir(sm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating state directory %q: %w", dir, err)
	}

	tmp := sm.filePath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("creating temp state file %q: %w", tmp, err)
	}

	w := bufio.NewWriter(f)
	for _, s := range states {
		if _, err := fmt.Fprintln(w, formatLine(s)); err != nil {
			f.Close()      //nolint:errcheck
			os.Remove(tmp) //nolint:errcheck
			return fmt.Errorf("writing state line for task %q: %w", s.TaskID, err)
		}
	}

	if err := w.Flush(); err != nil {
		f.Close()      //nolint:errcheck
		os.Remove(tmp) //nolint:errcheck
		return fmt.Errorf("flushing state file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmp) //nolint:errcheck
		return fmt.Errorf("closing temp state file: %w", err)
	}

	if err := os.Rename(tmp, sm.filePath); err != nil {
		os.Remove(tmp) //nolint:errcheck
		return fmt.Errorf("renaming temp state file to %q: %w", sm.filePath, err)
	}

	return nil
}
