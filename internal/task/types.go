package task

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	// StatusNotStarted indicates the task has not begun execution.
	StatusNotStarted TaskStatus = "not_started"

	// StatusInProgress indicates the task is currently being executed.
	StatusInProgress TaskStatus = "in_progress"

	// StatusCompleted indicates the task has finished successfully.
	StatusCompleted TaskStatus = "completed"

	// StatusBlocked indicates the task cannot proceed due to unmet dependencies.
	StatusBlocked TaskStatus = "blocked"

	// StatusSkipped indicates the task was intentionally skipped.
	StatusSkipped TaskStatus = "skipped"
)

// validStatuses is the set of all known TaskStatus values.
var validStatuses = map[TaskStatus]bool{
	StatusNotStarted: true,
	StatusInProgress: true,
	StatusCompleted:  true,
	StatusBlocked:    true,
	StatusSkipped:    true,
}

// Task represents a single implementation task parsed from a markdown spec file.
type Task struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Status       TaskStatus `json:"status"`
	Phase        int        `json:"phase"`
	Dependencies []string   `json:"dependencies"`
	SpecFile     string     `json:"spec_file"`
}

// Phase represents a group of related tasks executed together.
type Phase struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	StartTask string `json:"start_task"`
	EndTask   string `json:"end_task"`
}

// IsReady returns true if all dependencies are in the completed set.
// A task with no dependencies is always ready.
func (t *Task) IsReady(completedTasks map[string]bool) bool {
	for _, dep := range t.Dependencies {
		if !completedTasks[dep] {
			return false
		}
	}
	return true
}

// ValidStatus returns true if the status is a known TaskStatus value.
func ValidStatus(s TaskStatus) bool {
	return validStatuses[s]
}

// ValidStatuses returns all valid task status values.
func ValidStatuses() []TaskStatus {
	return []TaskStatus{StatusNotStarted, StatusInProgress, StatusCompleted, StatusBlocked, StatusSkipped}
}

// IsValid returns true if the status is a recognized value.
func (s TaskStatus) IsValid() bool {
	return validStatuses[s]
}
