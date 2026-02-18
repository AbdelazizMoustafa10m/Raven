package prd

import (
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// update is the golden-file update flag; run with -update to regenerate.
var update = flag.Bool("update", false, "update golden files")

// --- EpicBreakdown.Validate() tests ---

func TestEpicBreakdown_Validate_Valid(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{
				ID:                  "E-001",
				Title:               "Authentication System",
				Description:         "Handles user authentication and authorization.",
				PRDSections:         []string{"Section 3.1"},
				EstimatedTaskCount:  8,
				DependenciesOnEpics: []string{},
			},
			{
				ID:                  "E-002",
				Title:               "User Profiles",
				Description:         "User profile management.",
				PRDSections:         []string{"Section 4.1"},
				EstimatedTaskCount:  5,
				DependenciesOnEpics: []string{"E-001"},
			},
		},
	}

	errs := eb.Validate()
	assert.Nil(t, errs, "expected no validation errors for valid breakdown")
}

func TestEpicBreakdown_Validate_EmptyEpics(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{Epics: []Epic{}}
	errs := eb.Validate()
	require.Len(t, errs, 1)
	assert.Equal(t, "epics", errs[0].Field)
	assert.Contains(t, errs[0].Message, "must not be empty")
}

func TestEpicBreakdown_Validate_NilEpics(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{}
	errs := eb.Validate()
	require.Len(t, errs, 1)
	assert.Equal(t, "epics", errs[0].Field)
}

func TestEpicBreakdown_Validate_RequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		epic      Epic
		wantField string
		wantMsg   string
	}{
		{
			name: "missing id",
			epic: Epic{
				ID:          "",
				Title:       "Title",
				Description: "Description",
			},
			wantField: "epics[0].id",
			wantMsg:   "must not be empty",
		},
		{
			name: "missing title",
			epic: Epic{
				ID:          "E-001",
				Title:       "",
				Description: "Description",
			},
			wantField: "epics[0].title",
			wantMsg:   "must not be empty",
		},
		{
			name: "missing description",
			epic: Epic{
				ID:          "E-001",
				Title:       "Title",
				Description: "",
			},
			wantField: "epics[0].description",
			wantMsg:   "must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eb := &EpicBreakdown{Epics: []Epic{tt.epic}}
			errs := eb.Validate()
			require.NotEmpty(t, errs)
			found := false
			for _, e := range errs {
				if e.Field == tt.wantField {
					assert.Contains(t, e.Message, tt.wantMsg)
					found = true
					break
				}
			}
			assert.True(t, found, "expected error for field %q", tt.wantField)
		})
	}
}

func TestEpicBreakdown_Validate_InvalidIDFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
	}{
		{"no prefix", "001"},
		{"lowercase", "e-001"},
		{"too short", "E-01"},
		{"too long", "E-0001"},
		{"wrong separator", "E001"},
		{"letters in number", "E-ABC"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eb := &EpicBreakdown{
				Epics: []Epic{
					{ID: tt.id, Title: "T", Description: "D"},
				},
			}
			errs := eb.Validate()
			require.NotEmpty(t, errs)
			found := false
			for _, e := range errs {
				if e.Field == "epics[0].id" {
					assert.Contains(t, e.Message, "invalid format")
					found = true
					break
				}
			}
			assert.True(t, found, "expected invalid format error for ID %q", tt.id)
		})
	}
}

func TestEpicBreakdown_Validate_DuplicateIDs(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "First", Description: "First epic"},
			{ID: "E-001", Title: "Duplicate", Description: "Duplicate epic"},
		},
	}

	errs := eb.Validate()
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "epics[1].id" && strings.Contains(e.Message, "duplicate") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected duplicate ID error for epics[1].id")
}

func TestEpicBreakdown_Validate_NegativeTaskCount(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "T", Description: "D", EstimatedTaskCount: -1},
		},
	}

	errs := eb.Validate()
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "epics[0].estimated_task_count" {
			assert.Contains(t, e.Message, ">= 0")
			found = true
			break
		}
	}
	assert.True(t, found, "expected estimated_task_count error")
}

func TestEpicBreakdown_Validate_ZeroTaskCountIsValid(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "T", Description: "D", EstimatedTaskCount: 0},
		},
	}

	errs := eb.Validate()
	assert.Nil(t, errs, "zero estimated_task_count must be valid")
}

func TestEpicBreakdown_Validate_SelfReference(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{
				ID:                  "E-001",
				Title:               "T",
				Description:         "D",
				DependenciesOnEpics: []string{"E-001"},
			},
		},
	}

	errs := eb.Validate()
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "epics[0].dependencies_on_epics[0]" {
			assert.Contains(t, e.Message, "cannot depend on itself")
			found = true
			break
		}
	}
	assert.True(t, found, "expected self-reference error")
}

func TestEpicBreakdown_Validate_UnknownDependency(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{
				ID:                  "E-001",
				Title:               "T",
				Description:         "D",
				DependenciesOnEpics: []string{"E-999"},
			},
		},
	}

	errs := eb.Validate()
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "epics[0].dependencies_on_epics[0]" {
			assert.Contains(t, e.Message, "unknown epic ID")
			found = true
			break
		}
	}
	assert.True(t, found, "expected unknown dependency error")
}

// --- EpicTaskResult.Validate() tests ---

func TestEpicTaskResult_Validate_Valid(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{
				TempID:               "E001-T01",
				Title:                "Set up middleware",
				Description:         "Implement auth middleware.",
				AcceptanceCriteria:  []string{"Tokens are validated"},
				LocalDependencies:   []string{},
				CrossEpicDeps:       []string{"E-003:database-schema"},
				Effort:              "medium",
				Priority:            "must-have",
			},
			{
				TempID:              "E001-T02",
				Title:               "Login endpoint",
				Description:         "POST /auth/login endpoint.",
				AcceptanceCriteria: []string{"Returns JWT on success"},
				LocalDependencies:  []string{"E001-T01"},
				CrossEpicDeps:      []string{},
				Effort:             "small",
				Priority:           "must-have",
			},
		},
	}

	errs := etr.Validate([]string{"E-001", "E-003"})
	assert.Nil(t, errs, "expected no validation errors for valid result")
}

func TestEpicTaskResult_Validate_EmptyEpicID(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{EpicID: ""}
	errs := etr.Validate(nil)
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "epic_id" && strings.Contains(e.Message, "must not be empty") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected epic_id empty error")
}

func TestEpicTaskResult_Validate_InvalidEpicIDFormat(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{EpicID: "EPIC-1"}
	errs := etr.Validate(nil)
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "epic_id" && strings.Contains(e.Message, "invalid format") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected epic_id invalid format error")
}

func TestEpicTaskResult_Validate_RequiredTaskFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		task      TaskDef
		wantField string
		wantMsg   string
	}{
		{
			name:      "missing temp_id",
			task:      TaskDef{TempID: "", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			wantField: "tasks[0].temp_id",
			wantMsg:   "must not be empty",
		},
		{
			name:      "missing title",
			task:      TaskDef{TempID: "E001-T01", Title: "", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			wantField: "tasks[0].title",
			wantMsg:   "must not be empty",
		},
		{
			name:      "missing description",
			task:      TaskDef{TempID: "E001-T01", Title: "T", Description: "", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			wantField: "tasks[0].description",
			wantMsg:   "must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			etr := &EpicTaskResult{
				EpicID: "E-001",
				Tasks:  []TaskDef{tt.task},
			}
			errs := etr.Validate(nil)
			require.NotEmpty(t, errs)
			found := false
			for _, e := range errs {
				if e.Field == tt.wantField {
					assert.Contains(t, e.Message, tt.wantMsg)
					found = true
					break
				}
			}
			assert.True(t, found, "expected error for field %q", tt.wantField)
		})
	}
}

func TestEpicTaskResult_Validate_InvalidTempIDFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tempID string
	}{
		{"missing dash", "E001T01"},
		{"lowercase e", "e001-T01"},
		{"too short task", "E001-T1"},
		{"epic with dash", "E-001-T01"},
		{"letters in task", "E001-Tab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			etr := &EpicTaskResult{
				EpicID: "E-001",
				Tasks: []TaskDef{
					{TempID: tt.tempID, Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
				},
			}
			errs := etr.Validate(nil)
			require.NotEmpty(t, errs)
			found := false
			for _, e := range errs {
				if e.Field == "tasks[0].temp_id" {
					assert.Contains(t, e.Message, "invalid format")
					found = true
					break
				}
			}
			assert.True(t, found, "expected invalid format error for temp_id %q", tt.tempID)
		})
	}
}

func TestEpicTaskResult_Validate_DuplicateTempIDs(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
			{TempID: "E001-T01", Title: "Dup", Description: "Dup", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have"},
		},
	}

	errs := etr.Validate(nil)
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "tasks[1].temp_id" && strings.Contains(e.Message, "duplicate") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected duplicate temp_id error")
}

func TestEpicTaskResult_Validate_InvalidEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		effort string
	}{
		{"empty", ""},
		{"unknown", "extra-large"},
		{"capitalized", "Small"},
		{"partial", "med"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			etr := &EpicTaskResult{
				EpicID: "E-001",
				Tasks: []TaskDef{
					{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: tt.effort, Priority: "must-have"},
				},
			}
			errs := etr.Validate(nil)
			require.NotEmpty(t, errs)
			found := false
			for _, e := range errs {
				if e.Field == "tasks[0].effort" {
					assert.Contains(t, e.Message, "must be one of")
					found = true
					break
				}
			}
			assert.True(t, found, "expected effort error for value %q", tt.effort)
		})
	}
}

func TestEpicTaskResult_Validate_InvalidPriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		priority string
	}{
		{"empty", ""},
		{"unknown", "critical"},
		{"capitalized", "Must-Have"},
		{"partial", "must"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			etr := &EpicTaskResult{
				EpicID: "E-001",
				Tasks: []TaskDef{
					{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: tt.priority},
				},
			}
			errs := etr.Validate(nil)
			require.NotEmpty(t, errs)
			found := false
			for _, e := range errs {
				if e.Field == "tasks[0].priority" {
					assert.Contains(t, e.Message, "must be one of")
					found = true
					break
				}
			}
			assert.True(t, found, "expected priority error for value %q", tt.priority)
		})
	}
}

func TestEpicTaskResult_Validate_EmptyAcceptanceCriteria(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: nil, Effort: "small", Priority: "must-have"},
		},
	}

	errs := etr.Validate(nil)
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "tasks[0].acceptance_criteria" {
			assert.Contains(t, e.Message, "must not be empty")
			found = true
			break
		}
	}
	assert.True(t, found, "expected acceptance_criteria error")
}

func TestEpicTaskResult_Validate_LocalDependencySelfReference(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{
				TempID:            "E001-T01",
				Title:             "T",
				Description:       "D",
				AcceptanceCriteria: []string{"ac"},
				LocalDependencies: []string{"E001-T01"},
				Effort:            "small",
				Priority:          "must-have",
			},
		},
	}

	errs := etr.Validate(nil)
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "tasks[0].local_dependencies[0]" {
			assert.Contains(t, e.Message, "cannot depend on itself")
			found = true
			break
		}
	}
	assert.True(t, found, "expected self-reference error")
}

func TestEpicTaskResult_Validate_UnknownLocalDependency(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{
				TempID:            "E001-T01",
				Title:             "T",
				Description:       "D",
				AcceptanceCriteria: []string{"ac"},
				LocalDependencies: []string{"E001-T99"},
				Effort:            "small",
				Priority:          "must-have",
			},
		},
	}

	errs := etr.Validate(nil)
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "tasks[0].local_dependencies[0]" {
			assert.Contains(t, e.Message, "unknown temp_id")
			found = true
			break
		}
	}
	assert.True(t, found, "expected unknown local dependency error")
}

func TestEpicTaskResult_Validate_CrossEpicDepInvalidFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dep  string
	}{
		{"no colon", "E-003"},
		{"no label", "E-003:"},
		{"wrong epic format", "epic3:label"},
		{"empty string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// empty string has its own specific error message
			if tt.dep == "" {
				etr := &EpicTaskResult{
					EpicID: "E-001",
					Tasks: []TaskDef{
						{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have", CrossEpicDeps: []string{""}},
					},
				}
				errs := etr.Validate(nil)
				require.NotEmpty(t, errs)
				found := false
				for _, e := range errs {
					if e.Field == "tasks[0].cross_epic_dependencies[0]" && strings.Contains(e.Message, "must not be empty") {
						found = true
						break
					}
				}
				assert.True(t, found, "expected empty cross_epic_dep error")
				return
			}

			etr := &EpicTaskResult{
				EpicID: "E-001",
				Tasks: []TaskDef{
					{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: "must-have", CrossEpicDeps: []string{tt.dep}},
				},
			}
			errs := etr.Validate(nil)
			require.NotEmpty(t, errs)
			found := false
			for _, e := range errs {
				if e.Field == "tasks[0].cross_epic_dependencies[0]" {
					found = true
					break
				}
			}
			assert.True(t, found, "expected cross_epic_dep error for %q", tt.dep)
		})
	}
}

func TestEpicTaskResult_Validate_CrossEpicDepUnknownEpic(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{
				TempID:            "E001-T01",
				Title:             "T",
				Description:       "D",
				AcceptanceCriteria: []string{"ac"},
				CrossEpicDeps:     []string{"E-999:some-label"},
				Effort:            "small",
				Priority:          "must-have",
			},
		},
	}

	errs := etr.Validate([]string{"E-001", "E-002", "E-003"})
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "tasks[0].cross_epic_dependencies[0]" {
			assert.Contains(t, e.Message, "unknown epic ID")
			found = true
			break
		}
	}
	assert.True(t, found, "expected unknown cross-epic ID error")
}

func TestEpicTaskResult_Validate_CrossEpicDepNoValidationWithoutKnownEpics(t *testing.T) {
	t.Parallel()

	// When knownEpicIDs is empty/nil, cross-epic epic ID part should not be validated
	// (we don't know the universe of epics yet).
	etr := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{
				TempID:            "E001-T01",
				Title:             "T",
				Description:       "D",
				AcceptanceCriteria: []string{"ac"},
				CrossEpicDeps:     []string{"E-999:some-label"},
				Effort:            "small",
				Priority:          "must-have",
			},
		},
	}

	errs := etr.Validate(nil)
	// No error for unknown epic ID when knownEpicIDs is nil
	for _, e := range errs {
		assert.NotContains(t, e.Message, "unknown epic ID", "should not validate cross-epic epic IDs when knownEpicIDs is empty")
	}
}

// --- FormatValidationErrors tests ---

func TestFormatValidationErrors_Empty(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", FormatValidationErrors(nil))
	assert.Equal(t, "", FormatValidationErrors([]ValidationError{}))
}

func TestFormatValidationErrors_Single(t *testing.T) {
	t.Parallel()
	errs := []ValidationError{
		{Field: "epics[0].id", Message: "must not be empty"},
	}
	got := FormatValidationErrors(errs)
	assert.Equal(t, "1. [epics[0].id] must not be empty\n", got)
}

func TestFormatValidationErrors_Multiple(t *testing.T) {
	t.Parallel()
	errs := []ValidationError{
		{Field: "epics[0].id", Message: "must not be empty"},
		{Field: "epics[0].title", Message: "must not be empty"},
		{Field: "epics[1].id", Message: `invalid format "bad"; must match E-NNN (e.g., E-001)`},
	}
	got := FormatValidationErrors(errs)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	assert.Len(t, lines, 3)
	assert.True(t, strings.HasPrefix(lines[0], "1. "))
	assert.True(t, strings.HasPrefix(lines[1], "2. "))
	assert.True(t, strings.HasPrefix(lines[2], "3. "))
}

// --- ParseEpicBreakdown tests ---

func TestParseEpicBreakdown_ValidJSON(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/valid_epic_breakdown.json")
	require.NoError(t, err)

	eb, validErrs, err := ParseEpicBreakdown(data)
	require.NoError(t, err)
	require.Nil(t, validErrs, "expected no validation errors: %v", FormatValidationErrors(validErrs))
	require.NotNil(t, eb)
	assert.Len(t, eb.Epics, 3)
	assert.Equal(t, "E-001", eb.Epics[0].ID)
	assert.Equal(t, "Authentication System", eb.Epics[0].Title)
}

func TestParseEpicBreakdown_InvalidJSON(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/invalid_epic_breakdown.json")
	require.NoError(t, err)

	eb, validErrs, err := ParseEpicBreakdown(data)
	require.NoError(t, err, "parse should not fail on structurally valid JSON")
	require.NotNil(t, eb)
	require.NotEmpty(t, validErrs, "expected validation errors for invalid breakdown")
}

func TestParseEpicBreakdown_MalformedJSON(t *testing.T) {
	t.Parallel()
	_, _, err := ParseEpicBreakdown([]byte(`{"epics": [`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal epic breakdown")
}

func TestParseEpicBreakdown_SizeCapExceeded(t *testing.T) {
	t.Parallel()
	// Build a byte slice slightly over 10 MB.
	oversized := make([]byte, maxJSONSize+1)
	_, _, err := ParseEpicBreakdown(oversized)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

// --- ParseEpicTaskResult tests ---

func TestParseEpicTaskResult_ValidJSON(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/valid_epic_task_result.json")
	require.NoError(t, err)

	knownEpics := []string{"E-001", "E-003"}
	etr, validErrs, err := ParseEpicTaskResult(data, knownEpics)
	require.NoError(t, err)
	require.Nil(t, validErrs, "expected no validation errors: %v", FormatValidationErrors(validErrs))
	require.NotNil(t, etr)
	assert.Equal(t, "E-001", etr.EpicID)
	assert.Len(t, etr.Tasks, 3)
	assert.Equal(t, "E001-T01", etr.Tasks[0].TempID)
}

func TestParseEpicTaskResult_MalformedJSON(t *testing.T) {
	t.Parallel()
	_, _, err := ParseEpicTaskResult([]byte(`{"epic_id": "E-001", "tasks": [`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal epic task result")
}

func TestParseEpicTaskResult_SizeCapExceeded(t *testing.T) {
	t.Parallel()
	oversized := make([]byte, maxJSONSize+1)
	_, _, err := ParseEpicTaskResult(oversized, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

// --- Edge case tests ---

func TestEpicBreakdown_Validate_UnicodeFields(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{
				ID:          "E-001",
				Title:       "Système d'authentification",
				Description: "Gère l'authentification et l'autorisation des utilisateurs.",
			},
		},
	}

	errs := eb.Validate()
	assert.Nil(t, errs, "unicode characters in title and description should be valid")
}

func TestEpicBreakdown_Validate_LongDescription(t *testing.T) {
	t.Parallel()

	longDesc := strings.Repeat("a", 15000)
	eb := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "T", Description: longDesc},
		},
	}

	errs := eb.Validate()
	assert.Nil(t, errs, "very long description should be valid")
}

func TestEpicTaskResult_Validate_NullVsEmptyDependencies(t *testing.T) {
	t.Parallel()

	// Both nil and empty slice should be valid for optional dependency arrays.
	etrNil := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, LocalDependencies: nil, CrossEpicDeps: nil, Effort: "small", Priority: "must-have"},
		},
	}
	errs := etrNil.Validate(nil)
	assert.Nil(t, errs, "nil dependency slices should be valid")

	etrEmpty := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, LocalDependencies: []string{}, CrossEpicDeps: []string{}, Effort: "small", Priority: "must-have"},
		},
	}
	errs = etrEmpty.Validate(nil)
	assert.Nil(t, errs, "empty dependency slices should be valid")
}

// --- Round-trip tests ---

func TestEpicBreakdown_RoundTrip(t *testing.T) {
	t.Parallel()
	raw := `{"epics":[{"id":"E-001","title":"T","description":"D","prd_sections":["S1"],"estimated_task_count":3,"dependencies_on_epics":[]}]}`

	eb, validErrs, err := ParseEpicBreakdown([]byte(raw))
	require.NoError(t, err)
	require.Nil(t, validErrs)
	require.NotNil(t, eb)
	assert.Equal(t, "E-001", eb.Epics[0].ID)
	assert.Equal(t, 3, eb.Epics[0].EstimatedTaskCount)
	assert.Equal(t, []string{"S1"}, eb.Epics[0].PRDSections)
}

func TestEpicTaskResult_RoundTrip(t *testing.T) {
	t.Parallel()
	input := `{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"large","priority":"nice-to-have"}]}`

	etr, validErrs, err := ParseEpicTaskResult([]byte(input), nil)
	require.NoError(t, err)
	require.Nil(t, validErrs)
	require.NotNil(t, etr)
	assert.Equal(t, "E-001", etr.EpicID)
	assert.Equal(t, "large", etr.Tasks[0].Effort)
	assert.Equal(t, "nice-to-have", etr.Tasks[0].Priority)
}

// --- Additional acceptance-criteria tests ---

// TestEpicBreakdown_Validate_InvalidIDFormat_Epic1Style verifies the exact example
// cited in the task spec: "epic1" instead of "E-001".
func TestEpicBreakdown_Validate_InvalidIDFormat_Epic1Style(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{ID: "epic1", Title: "T", Description: "D"},
		},
	}

	errs := eb.Validate()
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "epics[0].id" && strings.Contains(e.Message, "invalid format") {
			found = true
			break
		}
	}
	assert.True(t, found, `expected invalid format error for ID "epic1"`)
}

// TestEpicBreakdown_Validate_EmptyDepEntry verifies that an empty string in
// DependenciesOnEpics is caught as a validation error.
func TestEpicBreakdown_Validate_EmptyDepEntry(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{
				ID:                  "E-001",
				Title:               "T",
				Description:         "D",
				DependenciesOnEpics: []string{""},
			},
		},
	}

	errs := eb.Validate()
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "epics[0].dependencies_on_epics[0]" && strings.Contains(e.Message, "must not be empty") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected empty dependency entry error")
}

// TestEpicTaskResult_Validate_EmptyLocalDepEntry verifies that an empty string
// in LocalDependencies is caught as a validation error.
func TestEpicTaskResult_Validate_EmptyLocalDepEntry(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{
				TempID:            "E001-T01",
				Title:             "T",
				Description:       "D",
				AcceptanceCriteria: []string{"ac"},
				LocalDependencies: []string{""},
				Effort:            "small",
				Priority:          "must-have",
			},
		},
	}

	errs := etr.Validate(nil)
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "tasks[0].local_dependencies[0]" && strings.Contains(e.Message, "must not be empty") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected empty local dependency entry error")
}

// TestParseEpicTaskResult_InvalidFixture verifies that the invalid_epic_task_result.json
// fixture parses without a structural error but produces multiple validation errors.
func TestParseEpicTaskResult_InvalidFixture(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/invalid_epic_task_result.json")
	require.NoError(t, err)

	etr, validErrs, err := ParseEpicTaskResult(data, []string{"E-001", "E-002"})
	require.NoError(t, err, "structurally valid JSON must not return a parse error")
	require.NotNil(t, etr)
	require.NotEmpty(t, validErrs, "expected validation errors for invalid task result")

	// Must catch the invalid epic_id format "EPIC-1".
	foundEpicIDErr := false
	for _, e := range validErrs {
		if e.Field == "epic_id" && strings.Contains(e.Message, "invalid format") {
			foundEpicIDErr = true
			break
		}
	}
	assert.True(t, foundEpicIDErr, "expected epic_id invalid format error")
}

// TestEpicBreakdown_ExtraUnknownFieldsIgnored verifies that unknown JSON keys do not
// cause a parse or validation error -- the decoder silently ignores them.
func TestEpicBreakdown_ExtraUnknownFieldsIgnored(t *testing.T) {
	t.Parallel()

	raw := `{
		"epics": [
			{
				"id": "E-001",
				"title": "Auth",
				"description": "Handles auth",
				"prd_sections": [],
				"estimated_task_count": 2,
				"dependencies_on_epics": [],
				"unknown_field_xyz": "should be ignored",
				"another_extra": 42
			}
		]
	}`

	eb, validErrs, err := ParseEpicBreakdown([]byte(raw))
	require.NoError(t, err)
	require.Nil(t, validErrs, "extra fields should be silently ignored, not produce validation errors")
	require.NotNil(t, eb)
	assert.Equal(t, "E-001", eb.Epics[0].ID)
}

// TestEpicTaskResult_ExtraUnknownFieldsIgnored verifies that unknown JSON keys in task
// definitions are silently ignored.
func TestEpicTaskResult_ExtraUnknownFieldsIgnored(t *testing.T) {
	t.Parallel()

	raw := `{
		"epic_id": "E-001",
		"extra_top_level": true,
		"tasks": [
			{
				"temp_id": "E001-T01",
				"title": "Task",
				"description": "Does something",
				"acceptance_criteria": ["passes tests"],
				"local_dependencies": [],
				"cross_epic_dependencies": [],
				"effort": "small",
				"priority": "must-have",
				"unknown_field": "ignored"
			}
		]
	}`

	etr, validErrs, err := ParseEpicTaskResult([]byte(raw), nil)
	require.NoError(t, err)
	require.Nil(t, validErrs, "extra fields should not produce validation errors")
	require.NotNil(t, etr)
	assert.Equal(t, "E001-T01", etr.Tasks[0].TempID)
}

// TestEpicBreakdown_Validate_UnicodeInAllFields exercises Unicode in all string fields.
func TestEpicBreakdown_Validate_UnicodeInAllFields(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{
				ID:          "E-001",
				Title:       "Подлинность（認証）système",
				Description: "Ελληνικά 日本語 한국어 العربية emoji: 🔑🛡️",
				PRDSections: []string{"§ 3.1 Überblick", "第2章"},
			},
		},
	}

	errs := eb.Validate()
	assert.Nil(t, errs, "unicode characters in all string fields must be valid")
}

// TestEpicTaskResult_Validate_UnicodeInAllFields exercises Unicode in all task string fields.
func TestEpicTaskResult_Validate_UnicodeInAllFields(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{
				TempID:             "E001-T01",
				Title:              "Tâche d'authentification — étape 1",
				Description:        "实现JWT验证中间件。処理フロー: 受信 → 検証 → 承認",
				AcceptanceCriteria: []string{"Токены JWT проверяются", "过期令牌被拒绝 with 401"},
				LocalDependencies:  nil,
				CrossEpicDeps:      nil,
				Effort:             "medium",
				Priority:           "must-have",
			},
		},
	}

	errs := etr.Validate(nil)
	assert.Nil(t, errs, "unicode characters in all task string fields must be valid")
}

// TestEpicTaskResult_Validate_LongDescription verifies that a description exceeding
// 10 000 characters is accepted by validation (no length cap on strings).
func TestEpicTaskResult_Validate_LongDescription(t *testing.T) {
	t.Parallel()

	longDesc := strings.Repeat("x", 15000)
	etr := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{
				TempID:             "E001-T01",
				Title:              "T",
				Description:        longDesc,
				AcceptanceCriteria: []string{"ac"},
				Effort:             "small",
				Priority:           "must-have",
			},
		},
	}

	errs := etr.Validate(nil)
	assert.Nil(t, errs, "long description should not be rejected by validation")
}

// TestEpicBreakdown_Validate_ZeroTaskCount_MultipleEpics ensures zero task count
// is valid even when multiple epics are present.
func TestEpicBreakdown_Validate_ZeroTaskCount_MultipleEpics(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "T1", Description: "D1", EstimatedTaskCount: 0},
			{ID: "E-002", Title: "T2", Description: "D2", EstimatedTaskCount: 0},
		},
	}

	errs := eb.Validate()
	assert.Nil(t, errs, "zero estimated_task_count on multiple epics must be valid")
}

// TestEpicBreakdown_Validate_AllErrorsAtOnce verifies that all validation rules
// are applied independently and multiple errors are collected in a single pass.
func TestEpicBreakdown_Validate_AllErrorsAtOnce(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			// epic[0]: bad id format, missing title, negative task count
			{ID: "badformat", Title: "", Description: "D", EstimatedTaskCount: -5},
			// epic[1]: missing description, self-dep
			{ID: "E-002", Title: "T2", Description: "", DependenciesOnEpics: []string{"E-002"}},
		},
	}

	errs := eb.Validate()
	require.NotEmpty(t, errs)

	fields := make(map[string]string, len(errs))
	for _, e := range errs {
		fields[e.Field] = e.Message
	}

	assert.Contains(t, fields, "epics[0].id", "should report invalid id format")
	assert.Contains(t, fields, "epics[0].title", "should report missing title")
	assert.Contains(t, fields, "epics[0].estimated_task_count", "should report negative task count")
	assert.Contains(t, fields, "epics[1].description", "should report missing description")
	assert.Contains(t, fields, "epics[1].dependencies_on_epics[0]", "should report self-dep")
}

// TestEpicTaskResult_Validate_AllErrorsAtOnce verifies that all task-level validation
// rules fire independently and accumulate without short-circuiting.
func TestEpicTaskResult_Validate_AllErrorsAtOnce(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{
		EpicID: "BADEPI",
		Tasks: []TaskDef{
			{
				TempID:             "bad-temp",
				Title:              "",
				Description:        "",
				AcceptanceCriteria: nil,
				LocalDependencies:  []string{"E001-T01"}, // references nonexistent temp_id
				CrossEpicDeps:      []string{"nocodon"},  // malformed cross-epic dep
				Effort:             "huge",
				Priority:           "optional",
			},
		},
	}

	errs := etr.Validate([]string{"E-001"})
	require.NotEmpty(t, errs)

	fields := make(map[string]string, len(errs))
	for _, e := range errs {
		fields[e.Field] = e.Message
	}

	assert.Contains(t, fields, "epic_id")
	assert.Contains(t, fields, "tasks[0].temp_id")
	assert.Contains(t, fields, "tasks[0].title")
	assert.Contains(t, fields, "tasks[0].description")
	assert.Contains(t, fields, "tasks[0].acceptance_criteria")
	assert.Contains(t, fields, "tasks[0].effort")
	assert.Contains(t, fields, "tasks[0].priority")
	assert.Contains(t, fields, "tasks[0].local_dependencies[0]")
	assert.Contains(t, fields, "tasks[0].cross_epic_dependencies[0]")
}

// TestFormatValidationErrors_OutputFormat verifies the exact numbered-list format
// produced by FormatValidationErrors.
func TestFormatValidationErrors_OutputFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		errs []ValidationError
		want string
	}{
		{
			name: "nil slice returns empty string",
			errs: nil,
			want: "",
		},
		{
			name: "empty slice returns empty string",
			errs: []ValidationError{},
			want: "",
		},
		{
			name: "single error with brackets in field",
			errs: []ValidationError{
				{Field: "epics[2].id", Message: "must not be empty"},
			},
			want: "1. [epics[2].id] must not be empty\n",
		},
		{
			name: "two errors are numbered starting at 1",
			errs: []ValidationError{
				{Field: "a", Message: "first"},
				{Field: "b", Message: "second"},
			},
			want: "1. [a] first\n2. [b] second\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatValidationErrors(tt.errs)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestParseEpicBreakdown_ExactlySizeCapIsAccepted ensures that input at exactly the
// maximum size boundary does not trigger the size-cap error (the error is strictly
// greater-than, not greater-than-or-equal).
func TestParseEpicBreakdown_ExactlySizeCapIsAccepted(t *testing.T) {
	t.Parallel()

	// Build a valid JSON payload padded to exactly maxJSONSize bytes.
	// We embed the padding as a value in a field that the unmarshaler ignores (unknown field).
	base := `{"epics":[{"id":"E-001","title":"T","description":"D","prd_sections":[],"estimated_task_count":0,"dependencies_on_epics":[]}],"_pad":"`
	suffix := `"}`
	padLen := maxJSONSize - len(base) - len(suffix)
	if padLen < 0 {
		t.Skip("base JSON exceeds maxJSONSize; adjust padding logic")
	}
	data := []byte(base + strings.Repeat("x", padLen) + suffix)
	require.Equal(t, maxJSONSize, len(data))

	// Should not error on the size cap -- it is exactly at the limit, not over.
	_, _, err := ParseEpicBreakdown(data)
	// We expect no size-cap error; validation/parse errors are acceptable.
	if err != nil {
		assert.NotContains(t, err.Error(), "exceeds maximum", "exactly-at-limit should not trigger size cap")
	}
}

// TestParseEpicTaskResult_ExactlySizeCapIsAccepted mirrors the breakdown boundary test.
func TestParseEpicTaskResult_ExactlySizeCapIsAccepted(t *testing.T) {
	t.Parallel()

	base := `{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"small","priority":"must-have"}],"_pad":"`
	suffix := `"}`
	padLen := maxJSONSize - len(base) - len(suffix)
	if padLen < 0 {
		t.Skip("base JSON exceeds maxJSONSize; adjust padding logic")
	}
	data := []byte(base + strings.Repeat("x", padLen) + suffix)
	require.Equal(t, maxJSONSize, len(data))

	_, _, err := ParseEpicTaskResult(data, nil)
	if err != nil {
		assert.NotContains(t, err.Error(), "exceeds maximum", "exactly-at-limit should not trigger size cap")
	}
}

// TestParseEpicBreakdown_SizeCap_OneByteOver verifies that input one byte above the
// maximum size limit is rejected.
func TestParseEpicBreakdown_SizeCap_OneByteOver(t *testing.T) {
	t.Parallel()

	data := make([]byte, maxJSONSize+1)
	_, _, err := ParseEpicBreakdown(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

// TestParseEpicTaskResult_SizeCap_OneByteOver verifies task result size-cap rejection.
func TestParseEpicTaskResult_SizeCap_OneByteOver(t *testing.T) {
	t.Parallel()

	data := make([]byte, maxJSONSize+1)
	_, _, err := ParseEpicTaskResult(data, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

// TestEpicBreakdown_Validate_MultipleErrorsOnOneEpic checks that a single epic with
// multiple problems reports all of them independently.
func TestEpicBreakdown_Validate_MultipleErrorsOnOneEpic(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{ID: "", Title: "", Description: "", EstimatedTaskCount: -3},
		},
	}

	errs := eb.Validate()
	require.NotEmpty(t, errs)

	fields := make(map[string]bool, len(errs))
	for _, e := range errs {
		fields[e.Field] = true
	}

	assert.True(t, fields["epics[0].id"], "should report missing id")
	assert.True(t, fields["epics[0].title"], "should report missing title")
	assert.True(t, fields["epics[0].description"], "should report missing description")
	assert.True(t, fields["epics[0].estimated_task_count"], "should report negative task count")
}

// TestEpicTaskResult_Validate_EmptyAcceptanceCriteriaSlice distinguishes between
// nil and empty-slice acceptance_criteria -- both must produce a validation error.
func TestEpicTaskResult_Validate_EmptyAcceptanceCriteriaSlice(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{
				TempID:             "E001-T01",
				Title:              "T",
				Description:        "D",
				AcceptanceCriteria: []string{}, // empty slice, not nil
				Effort:             "small",
				Priority:           "must-have",
			},
		},
	}

	errs := etr.Validate(nil)
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "tasks[0].acceptance_criteria" && strings.Contains(e.Message, "must not be empty") {
			found = true
			break
		}
	}
	assert.True(t, found, "empty acceptance_criteria slice must produce validation error")
}

// TestParseEpicBreakdown_NullEpicsField tests that {"epics": null} is treated the
// same as an empty epics array.
func TestParseEpicBreakdown_NullEpicsField(t *testing.T) {
	t.Parallel()

	_, validErrs, err := ParseEpicBreakdown([]byte(`{"epics": null}`))
	require.NoError(t, err)
	require.NotEmpty(t, validErrs, "null epics must produce a validation error")
	assert.Equal(t, "epics", validErrs[0].Field)
}

// TestEpicBreakdown_JSONMarshalRoundTrip verifies that an EpicBreakdown that passes
// validation can be marshalled to JSON and unmarshalled back to an identical value.
func TestEpicBreakdown_JSONMarshalRoundTrip(t *testing.T) {
	t.Parallel()

	original := &EpicBreakdown{
		Epics: []Epic{
			{
				ID:                  "E-001",
				Title:               "Auth",
				Description:         "Authentication",
				PRDSections:         []string{"3.1"},
				EstimatedTaskCount:  4,
				DependenciesOnEpics: []string{},
			},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	parsed, validErrs, err := ParseEpicBreakdown(data)
	require.NoError(t, err)
	require.Nil(t, validErrs)
	assert.Equal(t, original, parsed)
}

// TestEpicTaskResult_JSONMarshalRoundTrip verifies a TaskResult can make a full JSON round trip.
func TestEpicTaskResult_JSONMarshalRoundTrip(t *testing.T) {
	t.Parallel()

	original := &EpicTaskResult{
		EpicID: "E-002",
		Tasks: []TaskDef{
			{
				TempID:             "E002-T01",
				Title:              "User profile UI",
				Description:        "Build the profile settings page.",
				AcceptanceCriteria: []string{"Page loads", "Changes persist"},
				LocalDependencies:  []string{},
				CrossEpicDeps:      []string{"E-001:auth-middleware"},
				Effort:             "large",
				Priority:           "should-have",
			},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	parsed, validErrs, err := ParseEpicTaskResult(data, []string{"E-001", "E-002"})
	require.NoError(t, err)
	require.Nil(t, validErrs)
	assert.Equal(t, original, parsed)
}

// TestEpicBreakdown_Validate_ValidIDFormats tests every valid E-NNN boundary case.
func TestEpicBreakdown_Validate_ValidIDFormats(t *testing.T) {
	t.Parallel()

	validIDs := []string{"E-000", "E-001", "E-010", "E-099", "E-100", "E-999"}

	for _, id := range validIDs {
		id := id // capture range variable
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			eb := &EpicBreakdown{
				Epics: []Epic{{ID: id, Title: "T", Description: "D"}},
			}
			errs := eb.Validate()
			assert.Nil(t, errs, "ID %q should be valid", id)
		})
	}
}

// TestEpicTaskResult_Validate_ValidTempIDFormats tests every valid ENNN-TNN boundary.
func TestEpicTaskResult_Validate_ValidTempIDFormats(t *testing.T) {
	t.Parallel()

	validTempIDs := []string{"E000-T00", "E001-T01", "E010-T10", "E099-T99", "E999-T99"}

	for _, tid := range validTempIDs {
		tid := tid
		t.Run(tid, func(t *testing.T) {
			t.Parallel()
			etr := &EpicTaskResult{
				EpicID: "E-001",
				Tasks: []TaskDef{
					{
						TempID:             tid,
						Title:              "T",
						Description:        "D",
						AcceptanceCriteria: []string{"ac"},
						Effort:             "small",
						Priority:           "must-have",
					},
				},
			}
			errs := etr.Validate(nil)
			// Filter out any errors that are NOT about temp_id to isolate format check.
			for _, e := range errs {
				assert.NotEqual(t, "tasks[0].temp_id", e.Field, "TempID %q should be valid", tid)
			}
		})
	}
}

// TestEpicTaskResult_Validate_AllValidEffortValues confirms each valid effort value
// is accepted by the validator.
func TestEpicTaskResult_Validate_AllValidEffortValues(t *testing.T) {
	t.Parallel()

	for _, effort := range []string{"small", "medium", "large"} {
		effort := effort
		t.Run(effort, func(t *testing.T) {
			t.Parallel()
			etr := &EpicTaskResult{
				EpicID: "E-001",
				Tasks: []TaskDef{
					{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: effort, Priority: "must-have"},
				},
			}
			errs := etr.Validate(nil)
			for _, e := range errs {
				assert.NotEqual(t, "tasks[0].effort", e.Field, "effort %q should be valid", effort)
			}
		})
	}
}

// TestEpicTaskResult_Validate_AllValidPriorityValues confirms each valid priority
// value is accepted.
func TestEpicTaskResult_Validate_AllValidPriorityValues(t *testing.T) {
	t.Parallel()

	for _, priority := range []string{"must-have", "should-have", "nice-to-have"} {
		priority := priority
		t.Run(priority, func(t *testing.T) {
			t.Parallel()
			etr := &EpicTaskResult{
				EpicID: "E-001",
				Tasks: []TaskDef{
					{TempID: "E001-T01", Title: "T", Description: "D", AcceptanceCriteria: []string{"ac"}, Effort: "small", Priority: priority},
				},
			}
			errs := etr.Validate(nil)
			for _, e := range errs {
				assert.NotEqual(t, "tasks[0].priority", e.Field, "priority %q should be valid", priority)
			}
		})
	}
}

// TestValidationError_JSONSerializable confirms that ValidationError marshals and
// unmarshals cleanly -- important for retry-prompt augmentation usage.
func TestValidationError_JSONSerializable(t *testing.T) {
	t.Parallel()

	original := ValidationError{
		Field:   `epics[0].dependencies_on_epics[1]`,
		Message: `references unknown epic ID "E-999"`,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded ValidationError
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original, decoded)
}

// TestParseEpicBreakdown_EmptyJSON tests that an empty JSON object results in a
// validation error (empty epics array) rather than a parse error.
func TestParseEpicBreakdown_EmptyJSON(t *testing.T) {
	t.Parallel()

	eb, validErrs, err := ParseEpicBreakdown([]byte(`{}`))
	require.NoError(t, err)
	require.NotNil(t, eb)
	require.NotEmpty(t, validErrs, "empty object must produce validation error for missing epics")
}

// TestParseEpicTaskResult_EmptyJSON tests that an empty JSON object results in a
// validation error for the missing epic_id.
func TestParseEpicTaskResult_EmptyJSON(t *testing.T) {
	t.Parallel()

	etr, validErrs, err := ParseEpicTaskResult([]byte(`{}`), nil)
	require.NoError(t, err)
	require.NotNil(t, etr)
	require.NotEmpty(t, validErrs, "empty object must produce validation error for missing epic_id")
	found := false
	for _, e := range validErrs {
		if e.Field == "epic_id" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected epic_id error for empty JSON object")
}

// TestEpicTaskResult_Validate_CrossEpicDepWithKnownEpics verifies that a cross-epic dep
// referencing an epic that IS in knownEpicIDs does not produce an error.
func TestEpicTaskResult_Validate_CrossEpicDepWithKnownEpics(t *testing.T) {
	t.Parallel()

	etr := &EpicTaskResult{
		EpicID: "E-002",
		Tasks: []TaskDef{
			{
				TempID:             "E002-T01",
				Title:              "T",
				Description:        "D",
				AcceptanceCriteria: []string{"ac"},
				CrossEpicDeps:      []string{"E-001:foundation", "E-003:db-schema"},
				Effort:             "medium",
				Priority:           "should-have",
			},
		},
	}

	errs := etr.Validate([]string{"E-001", "E-002", "E-003"})
	for _, e := range errs {
		assert.NotEqual(t, "tasks[0].cross_epic_dependencies[0]", e.Field, "valid cross-epic dep should not produce error")
		assert.NotEqual(t, "tasks[0].cross_epic_dependencies[1]", e.Field, "valid cross-epic dep should not produce error")
	}
}

// TestEpicBreakdown_Validate_MultipleDepsOnSameEpic verifies that multiple valid
// cross-epic dependency entries on a single epic are all accepted.
func TestEpicBreakdown_Validate_MultipleDepsOnSameEpic(t *testing.T) {
	t.Parallel()

	eb := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "T1", Description: "D1"},
			{ID: "E-002", Title: "T2", Description: "D2"},
			{
				ID:                  "E-003",
				Title:               "T3",
				Description:         "D3",
				DependenciesOnEpics: []string{"E-001", "E-002"},
			},
		},
	}

	errs := eb.Validate()
	assert.Nil(t, errs, "multiple valid deps on a single epic must be accepted")
}

// --- Golden tests ---

// TestParseEpicBreakdown_Golden verifies the parsed EpicBreakdown from the fixture
// against a stable golden JSON snapshot.
func TestParseEpicBreakdown_Golden(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/valid_epic_breakdown.json")
	require.NoError(t, err)

	eb, validErrs, err := ParseEpicBreakdown(data)
	require.NoError(t, err)
	require.Nil(t, validErrs)

	goldenPath := "testdata/expected-output/epic_breakdown_golden.json"

	raw, err := json.MarshalIndent(eb, "", "  ")
	require.NoError(t, err)
	// Append a trailing newline so golden files are standard text files.
	got := append(raw, '\n')

	if *update {
		require.NoError(t, os.MkdirAll("testdata/expected-output", 0o755))
		require.NoError(t, os.WriteFile(goldenPath, got, 0o644))
		return
	}

	expected, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "golden file missing; run with -update to create")
	assert.Equal(t, string(expected), string(got))
}

// TestParseEpicTaskResult_Golden verifies the parsed EpicTaskResult from the fixture
// against a stable golden JSON snapshot.
func TestParseEpicTaskResult_Golden(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/valid_epic_task_result.json")
	require.NoError(t, err)

	etr, validErrs, err := ParseEpicTaskResult(data, []string{"E-001", "E-003"})
	require.NoError(t, err)
	require.Nil(t, validErrs)

	goldenPath := "testdata/expected-output/epic_task_result_golden.json"

	raw, err := json.MarshalIndent(etr, "", "  ")
	require.NoError(t, err)
	// Append a trailing newline so golden files are standard text files.
	got := append(raw, '\n')

	if *update {
		require.NoError(t, os.MkdirAll("testdata/expected-output", 0o755))
		require.NoError(t, os.WriteFile(goldenPath, got, 0o644))
		return
	}

	expected, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "golden file missing; run with -update to create")
	assert.Equal(t, string(expected), string(got))
}

// --- Benchmark tests ---

// BenchmarkEpicBreakdown_Validate benchmarks the hot validation path.
func BenchmarkEpicBreakdown_Validate(b *testing.B) {
	eb := &EpicBreakdown{
		Epics: []Epic{
			{ID: "E-001", Title: "Auth", Description: "Authentication system.", EstimatedTaskCount: 8, DependenciesOnEpics: []string{}},
			{ID: "E-002", Title: "Profiles", Description: "User profiles.", EstimatedTaskCount: 5, DependenciesOnEpics: []string{"E-001"}},
			{ID: "E-003", Title: "DB", Description: "Database schema.", EstimatedTaskCount: 6, DependenciesOnEpics: []string{}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eb.Validate()
	}
}

// BenchmarkEpicTaskResult_Validate benchmarks task-result validation with multiple tasks.
func BenchmarkEpicTaskResult_Validate(b *testing.B) {
	etr := &EpicTaskResult{
		EpicID: "E-001",
		Tasks: []TaskDef{
			{TempID: "E001-T01", Title: "Middleware", Description: "Auth middleware.", AcceptanceCriteria: []string{"Tokens validated"}, Effort: "medium", Priority: "must-have"},
			{TempID: "E001-T02", Title: "Login", Description: "Login endpoint.", AcceptanceCriteria: []string{"JWT returned"}, LocalDependencies: []string{"E001-T01"}, Effort: "small", Priority: "must-have"},
			{TempID: "E001-T03", Title: "Logout", Description: "Logout endpoint.", AcceptanceCriteria: []string{"Token revoked"}, LocalDependencies: []string{"E001-T01", "E001-T02"}, CrossEpicDeps: []string{"E-003:db-schema"}, Effort: "medium", Priority: "should-have"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		etr.Validate([]string{"E-001", "E-003"})
	}
}

// BenchmarkParseEpicBreakdown benchmarks full JSON unmarshaling + validation.
func BenchmarkParseEpicBreakdown(b *testing.B) {
	data, err := os.ReadFile("testdata/valid_epic_breakdown.json")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = ParseEpicBreakdown(data)
	}
}

// BenchmarkParseEpicTaskResult benchmarks full JSON unmarshaling + validation.
func BenchmarkParseEpicTaskResult(b *testing.B) {
	data, err := os.ReadFile("testdata/valid_epic_task_result.json")
	if err != nil {
		b.Fatal(err)
	}
	knownEpics := []string{"E-001", "E-003"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = ParseEpicTaskResult(data, knownEpics)
	}
}

// --- Fuzz tests ---

// FuzzParseEpicBreakdown fuzz-tests the JSON parsing path for epic breakdowns.
// It verifies that no input causes a panic and that the size cap is respected.
func FuzzParseEpicBreakdown(f *testing.F) {
	// Seed with known-good, known-bad, and edge-case inputs.
	f.Add([]byte(`{"epics":[]}`))
	f.Add([]byte(`{"epics":[{"id":"E-001","title":"T","description":"D","prd_sections":[],"estimated_task_count":0,"dependencies_on_epics":[]}]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"epics": null}`))
	f.Add([]byte(`not-json`))
	f.Add([]byte(``))
	f.Add([]byte(`{"epics":[{"id":"epic1","title":"","description":"","estimated_task_count":-1}]}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic.
		eb, _, err := ParseEpicBreakdown(data)

		// If within size cap and no error, the result must be non-nil.
		if len(data) <= maxJSONSize && err == nil {
			if eb == nil {
				t.Error("ParseEpicBreakdown returned nil result without an error")
			}
		}

		// If over the size cap, must return a size-cap error.
		if len(data) > maxJSONSize {
			if err == nil {
				t.Error("ParseEpicBreakdown must return an error for oversized input")
			}
		}
	})
}

// FuzzParseEpicTaskResult fuzz-tests the JSON parsing path for epic task results.
func FuzzParseEpicTaskResult(f *testing.F) {
	f.Add([]byte(`{"epic_id":"E-001","tasks":[]}`))
	f.Add([]byte(`{"epic_id":"E-001","tasks":[{"temp_id":"E001-T01","title":"T","description":"D","acceptance_criteria":["ac"],"local_dependencies":[],"cross_epic_dependencies":[],"effort":"small","priority":"must-have"}]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`not-json`))
	f.Add([]byte(``))
	f.Add([]byte(`{"epic_id":"BADEPI","tasks":[{"effort":"huge","priority":"optional"}]}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		etr, _, err := ParseEpicTaskResult(data, nil)

		if len(data) <= maxJSONSize && err == nil {
			if etr == nil {
				t.Error("ParseEpicTaskResult returned nil result without an error")
			}
		}

		if len(data) > maxJSONSize {
			if err == nil {
				t.Error("ParseEpicTaskResult must return an error for oversized input")
			}
		}
	})
}
