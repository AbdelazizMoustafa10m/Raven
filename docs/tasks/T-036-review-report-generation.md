# T-036: Review Report Generation (Markdown)

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-031, T-034 |
| Blocked By | T-031, T-034 |
| Blocks | T-038, T-039 |

## Goal
Implement the review report generator that takes a consolidated review and produces a structured markdown report. The report includes an executive summary, verdict, findings grouped by severity and file, per-agent breakdowns, consolidation statistics, and actionable next steps. This report is the primary human-readable output of the review pipeline and is also embedded in PR descriptions.

## Background
Per PRD Section 5.5, the review pipeline produces a unified review report in markdown format. This report serves multiple purposes: (1) direct developer consumption via terminal output, (2) inclusion in PR bodies (T-041), (3) input to the fix engine (T-038) which reads the findings. The report must be structured enough to be machine-parseable for the fix engine while also being human-readable.

## Technical Specifications
### Implementation Approach
Create `internal/review/report.go` containing a `ReportGenerator` that takes a `ConsolidatedReview`, `ConsolidationStats`, and `DiffResult`, and renders a markdown report using `text/template`. The template provides sections for summary, verdict, findings tables, per-agent results, and statistics. The generator also supports writing the report to a file and returning it as a string.

### Key Components
- **ReportGenerator**: Renders consolidated review data into markdown
- **ReportTemplate**: Embedded Go template for report formatting
- **ReportData**: Template data struct with all report sections
- **SeverityEmoji**: Maps severity levels to visual indicators for markdown

### API/Interface Contracts
```go
// internal/review/report.go

type ReportGenerator struct {
    tmpl   *template.Template
    logger *log.Logger
}

type ReportData struct {
    Verdict         Verdict
    VerdictEmoji    string
    TotalFindings   int
    CriticalCount   int
    HighCount       int
    MediumCount     int
    LowCount        int
    InfoCount       int
    Findings        []*Finding
    FindingsByFile  map[string][]*Finding
    FindingsBySeverity map[Severity][]*Finding
    AgentResults    []AgentReviewResult
    Stats           *ConsolidationStats
    DiffStats       DiffStats
    GeneratedAt     time.Time
}

func NewReportGenerator(logger *log.Logger) *ReportGenerator

// Generate produces a markdown report string from a consolidated review.
func (rg *ReportGenerator) Generate(
    consolidated *ConsolidatedReview,
    stats *ConsolidationStats,
    diffResult *DiffResult,
) (string, error)

// WriteToFile generates the report and writes it to the specified path.
func (rg *ReportGenerator) WriteToFile(
    path string,
    consolidated *ConsolidatedReview,
    stats *ConsolidationStats,
    diffResult *DiffResult,
) error
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| text/template | stdlib | Markdown report template rendering |
| embed | stdlib | Embedded default report template |
| os | stdlib | File I/O for writing report |
| time | stdlib | Timestamp for report generation |
| internal/review (T-031) | - | ConsolidatedReview, Finding, Verdict types |
| internal/review (T-034) | - | ConsolidationStats type |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Generates a valid markdown report from a ConsolidatedReview
- [ ] Report includes: verdict with visual indicator, summary statistics, findings table
- [ ] Findings are grouped by file with severity indicators
- [ ] Findings are also grouped by severity with counts
- [ ] Per-agent breakdown shows each agent's verdict and finding count
- [ ] Consolidation statistics show overlap rate and deduplication metrics
- [ ] Diff statistics (files changed, lines added/deleted) are included
- [ ] Report timestamp is included
- [ ] Report renders correctly in GitHub markdown (tables, headers, code blocks)
- [ ] WriteToFile creates parent directories if needed
- [ ] Empty findings (APPROVED with zero issues) produces a clean "no issues found" report
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- Generate report with 5 findings across 3 files: all findings appear, grouped correctly
- Generate report with zero findings and APPROVED verdict: clean pass report
- Generate report with BLOCKING verdict: appropriate severity indicator
- Findings sorted by severity within each file group
- Per-agent breakdown shows correct counts per agent
- Stats section shows correct overlap rate
- Report contains valid markdown (no broken tables, headers, links)
- WriteToFile creates file at specified path
- WriteToFile creates parent directories

### Integration Tests
- Generate report from a realistic consolidated review and verify markdown renders in a viewer

### Edge Cases to Handle
- Finding with empty suggestion: suggestion column omitted or shows "N/A"
- Finding with very long description: properly wrapped in markdown
- Many findings (100+): report should still be readable with a summary table
- File paths with special markdown characters (underscores, brackets)
- Agent name with special characters
- Zero agents in consolidated review (edge case, should not occur in practice)

## Implementation Notes
### Recommended Approach
1. Define the report template as an embedded `//go:embed` string
2. Template structure:
   ```markdown
   # Review Report
   
   ## Verdict: {{.VerdictEmoji}} {{.Verdict}}
   
   ## Summary
   | Metric | Count |
   | --- | --- |
   | Total Findings | {{.TotalFindings}} |
   ...
   
   ## Findings by File
   ### `{{$file}}`
   | Severity | Category | Line | Description | Suggestion |
   ...
   
   ## Agent Results
   ...
   
   ## Statistics
   ...
   ```
3. Group findings by file using a map in ReportData preparation
4. Use template functions for severity emoji mapping and markdown escaping
5. Write the rendered string to file using os.WriteFile with 0644 permissions

### Potential Pitfalls
- Markdown table cells cannot contain pipe characters (`|`) -- escape them in finding descriptions
- Markdown table cells cannot contain newlines -- replace with `<br>` or truncate
- Go template `range` over maps has non-deterministic order -- sort keys before rendering
- Verdict emoji should use unicode text, not actual emoji characters, for terminal compatibility (or use text indicators like `[PASS]`, `[FAIL]`, `[BLOCK]`)

### Security Considerations
- Finding descriptions may contain code snippets -- render them in code blocks to prevent markdown injection
- File paths in the report should be relative to the project root

## References
- [PRD Section 5.5 - Produces a unified review report (markdown)](docs/prd/PRD-Raven.md)
- [Go text/template documentation](https://pkg.go.dev/text/template)
- [GitHub Flavored Markdown spec](https://github.github.com/gfm/)
