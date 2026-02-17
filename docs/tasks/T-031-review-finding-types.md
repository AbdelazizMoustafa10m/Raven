# T-031: Review Finding Types and Schema

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 2-4hrs |
| Dependencies | T-004 |
| Blocked By | T-004 |
| Blocks | T-032, T-033, T-034, T-035, T-036, T-038, T-039 |

## Goal
Define the foundational data types for the review pipeline: findings, review results, verdicts, and review configuration. These types are consumed by every other component in the review subsystem -- diff generation, consolidation, report generation, and the fix engine.

## Background
Per PRD Section 5.5, each review agent produces structured JSON findings with severity, category, file, line, description, and suggestion fields. The review pipeline also requires a verdict type (`APPROVED`, `CHANGES_NEEDED`, `BLOCKING`) and a consolidated review result that aggregates findings from multiple agents. These types form the contract between all review pipeline components and must be defined before any other review work begins.

## Technical Specifications
### Implementation Approach
Create `internal/review/types.go` containing all shared types for the review subsystem. Use Go struct tags for JSON marshaling/unmarshaling since agent output arrives as JSON. Define verdict constants, severity levels, and finding categories as typed string constants for type safety. Include validation methods to verify agent output conforms to expectations.

### Key Components
- **Finding**: A single review finding with severity, category, file, line, description, and suggestion
- **ReviewResult**: Output from a single agent review pass containing findings and a verdict
- **Verdict**: Typed string constant for review verdicts (APPROVED, CHANGES_NEEDED, BLOCKING)
- **Severity**: Typed string constant for finding severity levels (info, low, medium, high, critical)
- **ReviewConfig**: Configuration subset loaded from `raven.toml` `[review]` section
- **AgentReviewResult**: Wraps ReviewResult with agent metadata (name, duration, errors)

### API/Interface Contracts
```go
// internal/review/types.go

type Verdict string

const (
    VerdictApproved      Verdict = "APPROVED"
    VerdictChangesNeeded Verdict = "CHANGES_NEEDED"
    VerdictBlocking      Verdict = "BLOCKING"
)

type Severity string

const (
    SeverityInfo     Severity = "info"
    SeverityLow      Severity = "low"
    SeverityMedium   Severity = "medium"
    SeverityHigh     Severity = "high"
    SeverityCritical Severity = "critical"
)

type Finding struct {
    Severity    Severity `json:"severity"`
    Category    string   `json:"category"`
    File        string   `json:"file"`
    Line        int      `json:"line"`
    Description string   `json:"description"`
    Suggestion  string   `json:"suggestion"`
    Agent       string   `json:"agent,omitempty"` // populated during consolidation
}

// DeduplicationKey returns the composite key used for deduplication: file:line:category
func (f *Finding) DeduplicationKey() string

type ReviewResult struct {
    Findings []Finding `json:"findings"`
    Verdict  Verdict   `json:"verdict"`
}

// Validate checks that the ReviewResult has valid severity and verdict values.
func (rr *ReviewResult) Validate() error

type AgentReviewResult struct {
    Agent    string
    Result   *ReviewResult
    Duration time.Duration
    Err      error
    RawOutput string // Full agent output for debugging
}

type ConsolidatedReview struct {
    Findings     []*Finding
    Verdict      Verdict
    AgentResults []AgentReviewResult
    TotalAgents  int
    Duration     time.Duration
}

type ReviewConfig struct {
    Extensions       string `toml:"extensions"`
    RiskPatterns     string `toml:"risk_patterns"`
    PromptsDir       string `toml:"prompts_dir"`
    RulesDir         string `toml:"rules_dir"`
    ProjectBriefFile string `toml:"project_brief_file"`
}

type ReviewMode string

const (
    ReviewModeAll   ReviewMode = "all"
    ReviewModeSplit ReviewMode = "split"
)

type ReviewOpts struct {
    Agents      []string
    Concurrency int
    Mode        ReviewMode
    BaseBranch  string
    DryRun      bool
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| encoding/json | stdlib | JSON marshaling/unmarshaling of review findings |
| fmt | stdlib | DeduplicationKey string formatting |
| time | stdlib | Duration tracking |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Finding struct marshals/unmarshals correctly to/from the JSON format specified in PRD Section 5.5
- [ ] DeduplicationKey() returns `file:line:category` composite key
- [ ] Verdict constants match PRD-specified values exactly (APPROVED, CHANGES_NEEDED, BLOCKING)
- [ ] Severity constants cover all levels: info, low, medium, high, critical
- [ ] ReviewResult.Validate() catches invalid severity values and invalid verdict values
- [ ] ReviewConfig struct matches `[review]` section in raven.toml
- [ ] All types have doc comments explaining their purpose
- [ ] Unit tests achieve 95% coverage

## Testing Requirements
### Unit Tests
- Finding JSON round-trip: marshal then unmarshal produces identical struct
- DeduplicationKey for Finding with file="main.go", line=42, category="security" returns "main.go:42:security"
- DeduplicationKey with empty category produces "main.go:42:"
- ReviewResult with valid fields passes Validate()
- ReviewResult with invalid severity fails Validate()
- ReviewResult with invalid verdict fails Validate()
- ReviewResult with empty findings and APPROVED verdict is valid
- Verdict comparison: VerdictBlocking is the most severe

### Integration Tests
- Parse sample JSON output matching PRD Section 5.5 agent output format

### Edge Cases to Handle
- Finding with line=0 (file-level finding, not line-specific)
- Finding with empty suggestion (not all findings have suggestions)
- Very long description or suggestion fields
- Category values with special characters or spaces
- Unknown severity values in agent output (should fail validation, not panic)

## Implementation Notes
### Recommended Approach
1. Define types in `internal/review/types.go`
2. Use typed string constants (not iota) for Verdict and Severity so JSON round-trip works naturally
3. Implement DeduplicationKey using `fmt.Sprintf("%s:%d:%s", f.File, f.Line, f.Category)`
4. Validate() should check severity against known values and verdict against known constants
5. Keep ReviewConfig aligned with the `[review]` TOML section in raven.toml

### Potential Pitfalls
- Do not use `iota` for Verdict or Severity -- they must serialize to their string values in JSON
- Agent output may include severity values in different cases ("High" vs "high") -- normalize in Validate() or document that normalization happens elsewhere
- The `Agent` field on Finding is empty in raw agent output and populated during consolidation -- use `omitempty` tag

### Security Considerations
- Validate file paths in findings to prevent path traversal if findings are used to read files downstream
- Cap the number of findings accepted from a single agent (prevent memory exhaustion from malformed output)

## References
- [PRD Section 5.5 - Multi-Agent Review Pipeline](docs/prd/PRD-Raven.md)
- [Go encoding/json documentation](https://pkg.go.dev/encoding/json)
