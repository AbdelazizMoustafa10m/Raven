# T-056: Define Epic and Task JSON Schemas with Validation

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | None |
| Blocked By | None |
| Blocks | T-057, T-059, T-061, T-065 |

## Goal
Define the JSON schema structures for epic-level and per-epic task breakdowns used throughout the PRD decomposition pipeline. Implement validation functions that enforce schema compliance on LLM-generated JSON, providing clear error messages for malformed output and enabling retry with augmented prompts.

## Background
Per PRD Section 5.8, the PRD decomposition workflow produces structured JSON at two levels: (1) an epic-level breakdown from the "shred" phase, and (2) per-epic task JSON from the "scatter" phase. Both schemas must be formally defined as Go structs with validation so that downstream merge/dedup/DAG phases operate on well-formed data. The PRD specifies exact JSON structures for both levels (see `epics` and `tasks` arrays in Section 5.8).

## Technical Specifications
### Implementation Approach
Define Go struct types in `internal/prd/schema.go` that mirror the JSON schemas from the PRD. Use `encoding/json` struct tags for marshaling/unmarshaling. Implement a `Validate()` method on each top-level type that checks required fields, valid enum values, and referential integrity (e.g., cross-epic dependency IDs reference real epics). Return structured validation errors that can be used to augment retry prompts.

### Key Components
- **EpicBreakdown**: Top-level struct for Phase 1 (shred) output containing `[]Epic`
- **Epic**: Individual epic with ID, title, description, PRD sections, estimated task count, and epic dependencies
- **EpicTaskResult**: Top-level struct for Phase 2 (scatter) output containing epic_id and `[]TaskDef`
- **TaskDef**: Individual task definition with temp_id, title, description, acceptance criteria, local dependencies, cross-epic dependencies, effort, and priority
- **ValidationError**: Structured error type with field path and message for retry prompt augmentation

### API/Interface Contracts
```go
// internal/prd/schema.go

type EpicBreakdown struct {
    Epics []Epic `json:"epics"`
}

type Epic struct {
    ID                  string   `json:"id"`                    // E-001 format
    Title               string   `json:"title"`
    Description         string   `json:"description"`
    PRDSections         []string `json:"prd_sections"`
    EstimatedTaskCount  int      `json:"estimated_task_count"`
    DependenciesOnEpics []string `json:"dependencies_on_epics"`
}

type EpicTaskResult struct {
    EpicID string    `json:"epic_id"`
    Tasks  []TaskDef `json:"tasks"`
}

type TaskDef struct {
    TempID               string   `json:"temp_id"`               // E001-T01 format
    Title                string   `json:"title"`
    Description          string   `json:"description"`
    AcceptanceCriteria   []string `json:"acceptance_criteria"`
    LocalDependencies    []string `json:"local_dependencies"`
    CrossEpicDeps        []string `json:"cross_epic_dependencies"`
    Effort               string   `json:"effort"`                // small, medium, large
    Priority             string   `json:"priority"`              // must-have, should-have, nice-to-have
}

type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

func (eb *EpicBreakdown) Validate() []ValidationError
func (etr *EpicTaskResult) Validate(knownEpicIDs []string) []ValidationError
func FormatValidationErrors(errs []ValidationError) string // For retry prompt augmentation
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| encoding/json | stdlib | JSON marshaling/unmarshaling |
| regexp | stdlib | ID format validation (E-NNN, ENNN-TNN) |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] EpicBreakdown and EpicTaskResult structs marshal/unmarshal correctly from PRD-specified JSON format
- [ ] Validate() catches: missing required fields, invalid ID formats, unknown effort/priority enums, self-referencing dependencies, references to nonexistent epics
- [ ] FormatValidationErrors produces human-readable text suitable for appending to retry prompts
- [ ] Round-trip test: marshal -> unmarshal -> validate on valid data passes
- [ ] Unit tests achieve 95% coverage for schema and validation code
- [ ] Invalid JSON inputs produce clear, actionable error messages

## Testing Requirements
### Unit Tests
- Valid epic breakdown JSON unmarshals and validates without errors
- Valid per-epic task JSON unmarshals and validates without errors
- Missing required fields (title, description, id) produce specific validation errors
- Invalid ID formats (e.g., "epic1" instead of "E-001") are caught
- Invalid enum values for effort and priority are caught
- Cross-epic dependency referencing nonexistent epic ID is caught
- Local dependency referencing nonexistent temp_id within same epic is caught
- Empty epics array produces validation error
- Duplicate epic IDs produce validation error

### Integration Tests
- Parse sample epic JSON files from testdata/

### Edge Cases to Handle
- Unicode characters in titles and descriptions
- Very long descriptions (>10000 chars)
- Zero estimated_task_count
- Empty dependency arrays vs null dependency arrays
- Extra unknown fields in JSON (should be ignored, not error)

## Implementation Notes
### Recommended Approach
1. Define structs with json tags matching PRD Section 5.8 exactly
2. Write Validate() using a builder pattern that accumulates []ValidationError
3. Use compiled regexes for ID format validation (E-NNN, ENNN-TNN)
4. FormatValidationErrors should produce numbered list format for prompt augmentation
5. Add testdata/ fixtures with valid and invalid JSON samples

### Potential Pitfalls
- Do not use `json:"required"` tags -- Go's encoding/json does not support required fields natively; check for zero values in Validate()
- Cross-epic dependencies use the format "E-003:database-schema" (epic_id:label) -- parse both parts
- The PRD shows `dependencies_on_epics` as an array that can be empty -- distinguish empty from null

### Security Considerations
- Validate JSON size before parsing (cap at 10MB to prevent memory exhaustion from LLM-generated output)
- Sanitize string fields if they will be used in file paths later (task file names)

## References
- [PRD Section 5.8 - Epic JSON schema](docs/prd/PRD-Raven.md)
- [Go encoding/json documentation](https://pkg.go.dev/encoding/json)
