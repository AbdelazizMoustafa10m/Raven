# T-039: PR Body Generation with AI Summary

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-021, T-031, T-036, T-037 |
| Blocked By | T-031, T-036, T-037 |
| Blocks | T-040 |

## Goal
Implement the PR body generation engine that produces a comprehensive, well-structured PR description. The body includes an AI-generated summary of changes, a list of tasks completed, review findings resolved, fix cycle results, and verification results. The generator optionally invokes an agent for the summary section and integrates with project PR templates.

## Background
Per PRD Section 5.7, the PR body includes a summary of changes, tasks completed, review findings resolved, and verification results. The summary is AI-generated via an agent call with a structured prompt that includes the diff, task specs, and review report. The PR body also integrates with the project's PR template (`.github/PULL_REQUEST_TEMPLATE.md`) if one exists. This component produces the body content; the actual `gh pr create` invocation is handled by T-040.

## Technical Specifications
### Implementation Approach
Create `internal/review/prbody.go` containing a `PRBodyGenerator` that collects all pipeline artifacts (diff stats, completed tasks, review report, fix report, verification report) and renders them into a markdown PR body. Optionally invoke an agent to generate a natural-language summary of the changes. If a PR template exists, integrate the generated content into the template's sections.

### Key Components
- **PRBodyGenerator**: Assembles all artifacts into a PR body
- **PRBodyData**: All data needed for the PR body
- **SummaryGenerator**: Invokes an agent to produce a natural-language summary
- **TemplateIntegrator**: Merges generated content with project PR template

### API/Interface Contracts
```go
// internal/review/prbody.go

type PRBodyGenerator struct {
    agent           agent.Agent  // for AI summary generation (optional)
    templatePath    string       // path to .github/PULL_REQUEST_TEMPLATE.md
    logger          *log.Logger
}

type PRBodyData struct {
    Summary             string              // AI-generated or manual summary
    TasksCompleted      []TaskSummary       // Tasks completed in this PR
    DiffStats           DiffStats           // Files changed, lines added/deleted
    ReviewVerdict       Verdict             // Final review verdict
    ReviewFindingsCount int                 // Total review findings
    ReviewReport        string              // Full review report markdown
    FixReport           *FixReport          // Fix cycle results (may be nil)
    VerificationReport  *VerificationReport // Verification results
    BranchName          string
    BaseBranch          string
    PhaseName           string              // If part of a phase pipeline
}

type TaskSummary struct {
    ID    string
    Title string
}

func NewPRBodyGenerator(
    agent agent.Agent,
    templatePath string,
    logger *log.Logger,
) *PRBodyGenerator

// Generate produces a markdown PR body string.
func (pg *PRBodyGenerator) Generate(ctx context.Context, data PRBodyData) (string, error)

// GenerateSummary uses an agent to produce a natural-language summary of changes.
func (pg *PRBodyGenerator) GenerateSummary(ctx context.Context, diff string, tasks []TaskSummary) (string, error)

// GenerateTitle produces a concise PR title from the tasks and phase information.
func (pg *PRBodyGenerator) GenerateTitle(data PRBodyData) string
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/agent (T-021) | - | Agent interface for summary generation |
| internal/review (T-031) | - | Finding, Verdict types |
| internal/review/report (T-036) | - | Review report content |
| internal/review/verify (T-037) | - | VerificationReport for results section |
| text/template | stdlib | PR body template rendering |
| os | stdlib | Reading PR template file |
| embed | stdlib | Default PR body template |
| strings | stdlib | Template integration, string manipulation |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Generates a markdown PR body with all required sections: summary, tasks, review, fixes, verification
- [ ] AI summary section uses an agent to produce a natural-language description of changes
- [ ] Falls back to a structured summary when agent is unavailable or fails
- [ ] Tasks completed section lists each task ID and title
- [ ] Review section includes verdict and finding count with link to full report
- [ ] Fix section shows number of fix cycles and final verification status
- [ ] Verification section shows pass/fail for each verification command
- [ ] Integrates with `.github/PULL_REQUEST_TEMPLATE.md` when present
- [ ] PR title is concise and descriptive (phase name + task range or summary)
- [ ] Generated body renders correctly in GitHub PR view
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- Generate body with all sections populated: all sections present in output
- Generate body with no review (review skipped): review section omitted
- Generate body with no fix cycles: fix section omitted
- Generate body with verification failures: failures shown with output excerpts
- GenerateSummary with mock agent: returns agent-generated summary
- GenerateSummary with agent failure: falls back to structured summary
- GenerateTitle with phase info: "Phase 2: Core Implementation (T-011 - T-020)"
- GenerateTitle without phase info: "Tasks T-007, T-008, T-009"
- PR template integration: generated content fills template sections
- PR template not found: uses default structure

### Integration Tests
- Generate full PR body from realistic pipeline data

### Edge Cases to Handle
- Very long review report (>10000 chars): truncate with "see full report" note
- Zero tasks completed (review-only PR): tasks section shows "No tasks"
- Agent summary produces markdown with conflicting headers: adjust heading levels
- PR template with custom sections: unknown sections preserved as-is
- FixReport is nil (fix was skipped): fix section cleanly omitted

## Implementation Notes
### Recommended Approach
1. Define PR body template as an embedded `//go:embed` string with sections:
   ```markdown
   ## Summary
   {{.Summary}}

   ## Tasks Completed
   {{range .TasksCompleted}}- {{.ID}}: {{.Title}}
   {{end}}

   ## Review Results
   **Verdict:** {{.ReviewVerdict}} | **Findings:** {{.ReviewFindingsCount}}
   {{if .ReviewReport}}
   <details><summary>Full Review Report</summary>
   {{.ReviewReport}}
   </details>
   {{end}}

   ## Fix Cycles
   ...

   ## Verification
   ...
   ```
2. Check for `.github/PULL_REQUEST_TEMPLATE.md` -- if present, try to fill known sections
3. For AI summary, construct a concise prompt with diff stats and task titles (not the full diff)
4. Render the template with PRBodyData

### Potential Pitfalls
- GitHub has a PR body size limit (65536 characters) -- truncate if necessary
- The review report in `<details>` blocks should not exceed GitHub's rendering limits
- AI summary generation may be slow -- consider a timeout
- PR template integration is best-effort -- do not fail if the template format is unexpected
- Heading levels in generated content must not conflict with the PR template structure

### Security Considerations
- Do not include full file contents or secrets in the PR body
- Sanitize agent-generated summary to prevent markdown injection
- Verification command output may contain file paths or error messages -- these are acceptable in PR body

## References
- [PRD Section 5.7 - PR Creation, AI-generated PR body](docs/prd/PRD-Raven.md)
- [GitHub PR body formatting](https://docs.github.com/en/pull-requests)
- [GitHub PR description size limits](https://docs.github.com/en/rest/pulls/pulls)
