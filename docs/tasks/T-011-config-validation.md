# T-011: Configuration Validation and Unknown Key Detection

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 4-6hrs |
| Dependencies | T-009, T-010 |
| Blocked By | T-009 |
| Blocks | T-012 |

## Goal
Implement configuration validation that checks resolved config values for correctness and completeness, plus unknown-key detection using `toml.MetaData.Undecoded()` to catch typos in `raven.toml`. Validation provides clear, actionable error messages that guide users to fix their configuration.

## Background
Per PRD Section 5.10, "Config validation at load time with clear error messages." The PRD also specifies using `MetaData.Undecoded()` to detect unknown keys (Section 6, Technical Considerations). This catches common mistakes like `[agents.cualde]` (typo for claude) or `taks_dir` (typo for tasks_dir).

Validation covers three categories: (1) structural validity (required fields present, correct types), (2) semantic validity (paths exist, regex patterns compile, referenced agents exist), and (3) unknown keys (fields in TOML that do not map to any config struct field).

## Technical Specifications
### Implementation Approach
Create `internal/config/validate.go` with a `Validate()` function that takes a resolved config and TOML metadata, runs all validation checks, and returns a structured list of warnings and errors. Errors are fatal (config is unusable), warnings are informational (config works but may have issues).

### Key Components
- **Validate()**: Main validation function returning structured results
- **ValidationResult**: Contains errors (fatal) and warnings (informational)
- **ValidationError**: Typed error with field path and message
- **UnknownKeyCheck**: Uses MetaData.Undecoded() for typo detection
- **Field validators**: Individual validation functions for each config section

### API/Interface Contracts
```go
// internal/config/validate.go

import "github.com/BurntSushi/toml"

// ValidationSeverity indicates whether a validation issue is an error or warning.
type ValidationSeverity string

const (
    SeverityError   ValidationSeverity = "error"
    SeverityWarning ValidationSeverity = "warning"
)

// ValidationIssue represents a single validation finding.
type ValidationIssue struct {
    Severity ValidationSeverity
    Field    string // dotted path, e.g., "project.name"
    Message  string
}

// ValidationResult holds all validation findings.
type ValidationResult struct {
    Issues []ValidationIssue
}

// HasErrors returns true if any issue has error severity.
func (vr *ValidationResult) HasErrors() bool

// HasWarnings returns true if any issue has warning severity.
func (vr *ValidationResult) HasWarnings() bool

// Errors returns only error-severity issues.
func (vr *ValidationResult) Errors() []ValidationIssue

// Warnings returns only warning-severity issues.
func (vr *ValidationResult) Warnings() []ValidationIssue

// Validate checks the resolved configuration for correctness.
// It performs structural validation, semantic validation, and unknown key detection.
//
// Parameters:
//   - cfg: the resolved configuration
//   - meta: TOML metadata from BurntSushi/toml (may be nil if no file loaded)
//
// Returns validation results. Check HasErrors() to determine if config is usable.
func Validate(cfg *Config, meta *toml.MetaData) *ValidationResult
```

**Validation rules:**

Errors (fatal):
- `project.name` must not be empty
- `project.language` must be a recognized value (go, typescript, python, rust, java, or empty)
- `project.verification_commands` entries must not be empty strings
- Agent `command` must not be empty if agent is defined
- Agent `effort` must be one of: low, medium, high (or empty)
- `review.extensions` must be a valid regex (if set)
- `review.risk_patterns` must be a valid regex (if set)
- Workflow `steps` must not be empty (if workflow is defined)
- Workflow transition keys must reference defined steps

Warnings (informational):
- Unknown keys detected via `MetaData.Undecoded()`
- `project.tasks_dir` directory does not exist
- `project.log_dir` directory does not exist
- Agent `prompt_template` file does not exist
- `review.prompts_dir` directory does not exist
- `review.project_brief_file` does not exist

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| github.com/BurntSushi/toml | v1.5.0 | MetaData.Undecoded() for unknown key detection |
| regexp | stdlib | Regex pattern validation |
| os | stdlib | File/directory existence checks |

## Acceptance Criteria
- [ ] `Validate()` with a valid config returns no errors
- [ ] `Validate()` with empty project.name returns an error for that field
- [ ] `Validate()` with invalid regex in review.extensions returns an error
- [ ] `Validate()` with unknown TOML keys returns warnings listing each unknown key
- [ ] `Validate()` with non-existent tasks_dir returns a warning (not error)
- [ ] `Validate()` with nil MetaData skips unknown-key detection (no panic)
- [ ] ValidationResult.HasErrors() correctly identifies presence of errors
- [ ] ValidationResult.Errors() filters to only error-severity issues
- [ ] ValidationResult.Warnings() filters to only warning-severity issues
- [ ] Validation messages include the field path and a human-readable description
- [ ] Unit tests achieve 95% coverage
- [ ] `go vet ./...` passes

## Testing Requirements
### Unit Tests
- Valid config: no errors, no warnings
- Empty project.name: error on "project.name"
- Invalid project.language: error on "project.language"
- Empty agent command: error on "agents.<name>.command"
- Invalid agent effort value: error on "agents.<name>.effort"
- Invalid review.extensions regex: error with regex compilation message
- Valid review.extensions regex: no error
- Unknown TOML keys: warnings listing each unknown key path
- Non-existent tasks_dir: warning on "project.tasks_dir"
- Nil MetaData: unknown-key check skipped, no panic
- Multiple errors: all collected (not short-circuit on first)
- Workflow with empty steps: error
- Workflow transition referencing undefined step: error

### Integration Tests
- Validate the actual project `raven.toml`: should have no errors

### Edge Cases to Handle
- Config with only defaults (no file loaded, meta is nil): should validate defaults cleanly
- Config with zero agents defined: valid (not all projects need agents configured)
- Config with zero workflows defined: valid (built-in workflows are used)
- Very complex regex patterns in review.extensions: should compile or report error
- Empty verification_commands array: valid (no commands to run)
- Agent name containing special characters: should not crash validation

## Implementation Notes
### Recommended Approach
1. Create `internal/config/validate.go`
2. Define the ValidationIssue, ValidationResult types
3. Implement `Validate()` as a sequential pipeline:
   a. Validate project section
   b. Validate each agent section
   c. Validate review section
   d. Validate each workflow section
   e. Check unknown keys via MetaData.Undecoded()
4. Each validation step appends issues to the result
5. File/directory existence checks use `os.Stat()` -- do not create missing directories
6. Create `internal/config/validate_test.go` with table-driven tests
7. Verify: `go build ./... && go vet ./... && go test ./internal/config/...`

### Potential Pitfalls
- `MetaData.Undecoded()` returns `[]toml.Key` where each key is a slice of strings representing the dotted path. Join with "." for display.
- Do NOT validate file/directory existence as errors -- they are warnings because the user may create them later. Only structural and semantic issues are errors.
- Regex compilation errors should include the original pattern in the error message for debugging.
- Do not panic on any validation input -- all validation functions must be safe with zero values.
- The `MetaData` parameter may be nil when config was loaded from defaults only (no file). Guard against nil pointer.

### Security Considerations
- Validation error messages should not expose full file system paths beyond the project directory
- Regex patterns from config could be crafted to cause ReDoS (regex denial of service) -- use `regexp.Compile` with a timeout or accept this risk for a dev tool

## References
- [BurntSushi/toml MetaData.Undecoded()](https://pkg.go.dev/github.com/BurntSushi/toml#MetaData.Undecoded)
- [PRD Section 5.10 - Config validation at load time](docs/prd/PRD-Raven.md)
- [PRD Section 6 - MetaData.Undecoded() for unknown-key detection](docs/prd/PRD-Raven.md)