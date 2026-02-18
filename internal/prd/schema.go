package prd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// maxJSONSize is the maximum allowed size for LLM-generated JSON input (10 MB).
const maxJSONSize = 10 * 1024 * 1024

// Pre-compiled regexes for ID format validation.
var (
	// reEpicID matches the E-NNN format (e.g., E-001, E-042).
	reEpicID = regexp.MustCompile(`^E-\d{3}$`)

	// reTempID matches the ENNN-TNN format (e.g., E001-T01, E042-T12).
	reTempID = regexp.MustCompile(`^E\d{3}-T\d{2}$`)

	// reCrossEpicDepEpicPart matches the "E-NNN:label" cross-epic dependency format.
	// The label must contain at least one non-whitespace character.
	reCrossEpicDepEpicPart = regexp.MustCompile(`^(E-\d{3}):.+`)
)

// Valid enum value sets for task fields.
var (
	validEffortValues = map[string]bool{
		"small":  true,
		"medium": true,
		"large":  true,
	}

	validPriorityValues = map[string]bool{
		"must-have":    true,
		"should-have":  true,
		"nice-to-have": true,
	}
)

// EpicBreakdown is the top-level struct for Phase 1 (shred) output containing a list of epics.
// It maps to the JSON produced by the LLM during PRD decomposition.
type EpicBreakdown struct {
	Epics []Epic `json:"epics"`
}

// Epic represents a single epic within an EpicBreakdown.
type Epic struct {
	// ID is the unique epic identifier in E-NNN format (e.g., E-001).
	ID string `json:"id"`
	// Title is the short human-readable name for the epic.
	Title string `json:"title"`
	// Description is a longer explanation of the epic's scope.
	Description string `json:"description"`
	// PRDSections lists the PRD section references covered by this epic.
	PRDSections []string `json:"prd_sections"`
	// EstimatedTaskCount is the expected number of tasks in this epic; must be >= 0.
	EstimatedTaskCount int `json:"estimated_task_count"`
	// DependenciesOnEpics lists the IDs of other epics this epic depends on.
	DependenciesOnEpics []string `json:"dependencies_on_epics"`
}

// EpicTaskResult is the top-level struct for Phase 2 (scatter) output containing per-epic task definitions.
type EpicTaskResult struct {
	// EpicID identifies which epic these tasks belong to; must be in E-NNN format.
	EpicID string `json:"epic_id"`
	// Tasks is the list of task definitions for this epic.
	Tasks []TaskDef `json:"tasks"`
}

// TaskDef represents a single task definition within an EpicTaskResult.
type TaskDef struct {
	// TempID is the temporary task identifier in ENNN-TNN format (e.g., E001-T01).
	TempID string `json:"temp_id"`
	// Title is the short human-readable name for the task.
	Title string `json:"title"`
	// Description explains what the task implements.
	Description string `json:"description"`
	// AcceptanceCriteria lists the conditions that must be met for the task to be complete.
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	// LocalDependencies lists temp_ids of other tasks within the same epic that must complete first.
	LocalDependencies []string `json:"local_dependencies"`
	// CrossEpicDeps lists cross-epic dependency references in "E-NNN:label" format.
	CrossEpicDeps []string `json:"cross_epic_dependencies"`
	// Effort is the size estimate; must be one of: "small", "medium", "large".
	Effort string `json:"effort"`
	// Priority is the importance classification; must be one of: "must-have", "should-have", "nice-to-have".
	Priority string `json:"priority"`
}

// ValidationError represents a single validation finding with a field path and human-readable message.
// It is designed to be serializable for use in retry prompt augmentation.
type ValidationError struct {
	// Field is the dotted path to the invalid field (e.g., "epics[0].id").
	Field string `json:"field"`
	// Message describes the validation failure in human-readable terms.
	Message string `json:"message"`
}

// Validate checks the EpicBreakdown for correctness.
//
// Rules enforced:
//   - epics array must not be empty
//   - each epic must have non-empty id, title, description
//   - epic id must match E-NNN format
//   - duplicate epic IDs are not allowed
//   - estimated_task_count must be >= 0
//   - dependencies_on_epics entries must reference valid epic IDs within the same breakdown (no self-reference, no unknown IDs)
//
// Returns a slice of ValidationError; returns nil if the breakdown is valid.
func (eb *EpicBreakdown) Validate() []ValidationError {
	var errs []ValidationError

	if len(eb.Epics) == 0 {
		errs = append(errs, ValidationError{
			Field:   "epics",
			Message: "must not be empty",
		})
		return errs
	}

	// Build a set of known epic IDs for referential integrity checks.
	epicIDs := make(map[string]bool, len(eb.Epics))
	for _, epic := range eb.Epics {
		if epic.ID != "" {
			epicIDs[epic.ID] = true
		}
	}

	// Track seen IDs for duplicate detection.
	seen := make(map[string]bool, len(eb.Epics))

	for i, epic := range eb.Epics {
		prefix := fmt.Sprintf("epics[%d]", i)

		if epic.ID == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".id",
				Message: "must not be empty",
			})
		} else {
			if !reEpicID.MatchString(epic.ID) {
				errs = append(errs, ValidationError{
					Field:   prefix + ".id",
					Message: fmt.Sprintf("invalid format %q; must match E-NNN (e.g., E-001)", epic.ID),
				})
			}
			if seen[epic.ID] {
				errs = append(errs, ValidationError{
					Field:   prefix + ".id",
					Message: fmt.Sprintf("duplicate epic ID %q", epic.ID),
				})
			}
			seen[epic.ID] = true
		}

		if epic.Title == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".title",
				Message: "must not be empty",
			})
		}

		if epic.Description == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".description",
				Message: "must not be empty",
			})
		}

		if epic.EstimatedTaskCount < 0 {
			errs = append(errs, ValidationError{
				Field:   prefix + ".estimated_task_count",
				Message: fmt.Sprintf("must be >= 0, got %d", epic.EstimatedTaskCount),
			})
		}

		for j, dep := range epic.DependenciesOnEpics {
			depPrefix := fmt.Sprintf("%s.dependencies_on_epics[%d]", prefix, j)

			if dep == "" {
				errs = append(errs, ValidationError{
					Field:   depPrefix,
					Message: "must not be empty",
				})
				continue
			}

			if dep == epic.ID {
				errs = append(errs, ValidationError{
					Field:   depPrefix,
					Message: fmt.Sprintf("epic %q cannot depend on itself", epic.ID),
				})
				continue
			}

			if !epicIDs[dep] {
				errs = append(errs, ValidationError{
					Field:   depPrefix,
					Message: fmt.Sprintf("references unknown epic ID %q", dep),
				})
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// Validate checks the EpicTaskResult for correctness.
//
// Parameters:
//   - knownEpicIDs: the set of epic IDs from the EpicBreakdown used to validate cross_epic_dependencies
//
// Rules enforced:
//   - epic_id must be non-empty and match E-NNN format
//   - each task must have non-empty temp_id, title, description
//   - temp_id must match ENNN-TNN format
//   - duplicate temp_ids are not allowed
//   - effort must be one of: "small", "medium", "large"
//   - priority must be one of: "must-have", "should-have", "nice-to-have"
//   - acceptance_criteria should not be empty (produces a validation error)
//   - local_dependencies must reference valid temp_ids within the same result (no self-reference)
//   - cross_epic_dependencies use "E-NNN:label" format; the epic ID part is validated against knownEpicIDs
//
// Returns a slice of ValidationError; returns nil if the result is valid.
func (etr *EpicTaskResult) Validate(knownEpicIDs []string) []ValidationError {
	var errs []ValidationError

	// Build a set of known epic IDs for cross-epic dependency validation.
	epicIDSet := make(map[string]bool, len(knownEpicIDs))
	for _, id := range knownEpicIDs {
		epicIDSet[id] = true
	}

	if etr.EpicID == "" {
		errs = append(errs, ValidationError{
			Field:   "epic_id",
			Message: "must not be empty",
		})
	} else if !reEpicID.MatchString(etr.EpicID) {
		errs = append(errs, ValidationError{
			Field:   "epic_id",
			Message: fmt.Sprintf("invalid format %q; must match E-NNN (e.g., E-001)", etr.EpicID),
		})
	}

	// Build a set of known temp_ids for local_dependencies validation.
	tempIDs := make(map[string]bool, len(etr.Tasks))
	for _, task := range etr.Tasks {
		if task.TempID != "" {
			tempIDs[task.TempID] = true
		}
	}

	// Track seen temp_ids for duplicate detection.
	seen := make(map[string]bool, len(etr.Tasks))

	for i, task := range etr.Tasks {
		prefix := fmt.Sprintf("tasks[%d]", i)

		if task.TempID == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".temp_id",
				Message: "must not be empty",
			})
		} else {
			if !reTempID.MatchString(task.TempID) {
				errs = append(errs, ValidationError{
					Field:   prefix + ".temp_id",
					Message: fmt.Sprintf("invalid format %q; must match ENNN-TNN (e.g., E001-T01)", task.TempID),
				})
			}
			if seen[task.TempID] {
				errs = append(errs, ValidationError{
					Field:   prefix + ".temp_id",
					Message: fmt.Sprintf("duplicate temp_id %q", task.TempID),
				})
			}
			seen[task.TempID] = true
		}

		if task.Title == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".title",
				Message: "must not be empty",
			})
		}

		if task.Description == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".description",
				Message: "must not be empty",
			})
		}

		if len(task.AcceptanceCriteria) == 0 {
			errs = append(errs, ValidationError{
				Field:   prefix + ".acceptance_criteria",
				Message: "must not be empty; provide at least one acceptance criterion",
			})
		}

		if !validEffortValues[task.Effort] {
			errs = append(errs, ValidationError{
				Field:   prefix + ".effort",
				Message: fmt.Sprintf("invalid value %q; must be one of: small, medium, large", task.Effort),
			})
		}

		if !validPriorityValues[task.Priority] {
			errs = append(errs, ValidationError{
				Field:   prefix + ".priority",
				Message: fmt.Sprintf("invalid value %q; must be one of: must-have, should-have, nice-to-have", task.Priority),
			})
		}

		for j, dep := range task.LocalDependencies {
			depPrefix := fmt.Sprintf("%s.local_dependencies[%d]", prefix, j)

			if dep == "" {
				errs = append(errs, ValidationError{
					Field:   depPrefix,
					Message: "must not be empty",
				})
				continue
			}

			if dep == task.TempID {
				errs = append(errs, ValidationError{
					Field:   depPrefix,
					Message: fmt.Sprintf("task %q cannot depend on itself", task.TempID),
				})
				continue
			}

			if !tempIDs[dep] {
				errs = append(errs, ValidationError{
					Field:   depPrefix,
					Message: fmt.Sprintf("references unknown temp_id %q within this epic", dep),
				})
			}
		}

		for j, crossDep := range task.CrossEpicDeps {
			depPrefix := fmt.Sprintf("%s.cross_epic_dependencies[%d]", prefix, j)

			if crossDep == "" {
				errs = append(errs, ValidationError{
					Field:   depPrefix,
					Message: "must not be empty",
				})
				continue
			}

			matches := reCrossEpicDepEpicPart.FindStringSubmatch(crossDep)
			if matches == nil {
				errs = append(errs, ValidationError{
					Field:   depPrefix,
					Message: fmt.Sprintf("invalid format %q; must match E-NNN:label (e.g., E-003:database-schema)", crossDep),
				})
				continue
			}

			epicIDPart := matches[1]
			if len(knownEpicIDs) > 0 && !epicIDSet[epicIDPart] {
				errs = append(errs, ValidationError{
					Field:   depPrefix,
					Message: fmt.Sprintf("references unknown epic ID %q in cross-epic dependency %q", epicIDPart, crossDep),
				})
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// FormatValidationErrors formats a slice of ValidationError as a numbered list suitable
// for appending to LLM retry prompts. Returns an empty string if errs is empty.
func FormatValidationErrors(errs []ValidationError) string {
	if len(errs) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, e := range errs {
		fmt.Fprintf(&sb, "%d. [%s] %s\n", i+1, e.Field, e.Message)
	}
	return sb.String()
}

// ParseEpicBreakdown parses JSON data into an EpicBreakdown, enforcing a 10 MB size cap.
// It unmarshals the JSON and validates the result, returning both the parsed value and
// any validation errors. Returns an error only for I/O or structural JSON failures.
func ParseEpicBreakdown(data []byte) (*EpicBreakdown, []ValidationError, error) {
	if len(data) > maxJSONSize {
		return nil, nil, fmt.Errorf("JSON input size %d bytes exceeds maximum of %d bytes", len(data), maxJSONSize)
	}

	var eb EpicBreakdown
	if err := json.Unmarshal(data, &eb); err != nil {
		return nil, nil, fmt.Errorf("unmarshal epic breakdown: %w", err)
	}

	validationErrs := eb.Validate()
	return &eb, validationErrs, nil
}

// ParseEpicTaskResult parses JSON data into an EpicTaskResult, enforcing a 10 MB size cap.
// It unmarshals the JSON and validates the result against the provided known epic IDs.
// Returns an error only for I/O or structural JSON failures.
func ParseEpicTaskResult(data []byte, knownEpicIDs []string) (*EpicTaskResult, []ValidationError, error) {
	if len(data) > maxJSONSize {
		return nil, nil, fmt.Errorf("JSON input size %d bytes exceeds maximum of %d bytes", len(data), maxJSONSize)
	}

	var etr EpicTaskResult
	if err := json.Unmarshal(data, &etr); err != nil {
		return nil, nil, fmt.Errorf("unmarshal epic task result: %w", err)
	}

	validationErrs := etr.Validate(knownEpicIDs)
	return &etr, validationErrs, nil
}
