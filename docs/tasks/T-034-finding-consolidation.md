# T-034: Finding Consolidation and Deduplication

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-031 |
| Blocked By | T-031 |
| Blocks | T-035, T-036, T-038 |

## Goal
Implement the finding consolidation engine that takes review results from multiple agents, deduplicates findings by composite key (file:line:category), escalates severity when multiple agents report the same issue, aggregates verdicts, and produces a unified consolidated review. This is the core merge step of the multi-agent review pipeline.

## Background
Per PRD Section 5.5, consolidation deduplicates findings by `file+line+category` composite key and escalates severity on duplicates. The verdict aggregation rule is: if any agent says `BLOCKING`, the final verdict is `BLOCKING`. The consolidation engine receives `[]AgentReviewResult` from the parallel review orchestrator and produces a single `ConsolidatedReview` with deduplicated findings and an aggregated verdict. Deduplication uses a `map[string]*Finding` keyed by the composite key for O(n) performance.

## Technical Specifications
### Implementation Approach
Create `internal/review/consolidate.go` containing a `Consolidator` that processes agent review results. It iterates through all findings from all agents, groups them by deduplication key, merges duplicates by escalating severity and combining descriptions/suggestions, tags each finding with the agent(s) that reported it, and computes the final verdict. The consolidator also generates summary statistics (findings per severity, per agent, overlap rate).

### Key Components
- **Consolidator**: Merges findings from multiple agents into a unified set
- **SeverityEscalation**: Logic for promoting severity when multiple agents report the same finding
- **VerdictAggregator**: Combines per-agent verdicts into a final verdict
- **ConsolidationStats**: Summary of the merge process (total input, deduplicated, overlap rate)

### API/Interface Contracts
```go
// internal/review/consolidate.go

type Consolidator struct {
    logger *log.Logger
}

type ConsolidationStats struct {
    TotalInputFindings  int
    UniqueFindings      int
    DuplicatesRemoved   int
    SeverityEscalations int
    OverlapRate         float64 // percentage of findings reported by 2+ agents
    FindingsPerAgent    map[string]int
    FindingsPerSeverity map[Severity]int
}

func NewConsolidator(logger *log.Logger) *Consolidator

// Consolidate merges findings from multiple agent reviews into a single
// deduplicated, severity-escalated result with an aggregated verdict.
func (c *Consolidator) Consolidate(results []AgentReviewResult) (*ConsolidatedReview, *ConsolidationStats)

// AggregateVerdicts computes the final verdict from per-agent verdicts.
// BLOCKING > CHANGES_NEEDED > APPROVED
func AggregateVerdicts(verdicts []Verdict) Verdict

// EscalateSeverity returns the higher of two severity levels.
func EscalateSeverity(a, b Severity) Severity

// severityRank returns a numeric rank for severity comparison.
func severityRank(s Severity) int
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/review (T-031) | - | Finding, ReviewResult, AgentReviewResult, Verdict, Severity types |
| fmt | stdlib | String formatting for merged descriptions |
| strings | stdlib | Combining descriptions from multiple agents |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Deduplicates findings by `file:line:category` composite key using O(n) map-based approach
- [ ] Severity escalation: when 2 agents report same finding, severity is promoted to the higher of the two
- [ ] Agent attribution: each finding tracks which agent(s) reported it
- [ ] Verdict aggregation: BLOCKING beats CHANGES_NEEDED beats APPROVED
- [ ] Findings from agents with errors (Err != nil) are excluded, but their verdict is treated as CHANGES_NEEDED
- [ ] ConsolidationStats accurately tracks input, output, duplicates, overlap rate
- [ ] Merged descriptions combine unique insights from multiple agents
- [ ] Order of findings in output: sorted by severity (critical first), then by file path, then by line number
- [ ] Unit tests achieve 95% coverage

## Testing Requirements
### Unit Tests
- Two agents report same finding (same file, line, category): deduplicated to one with escalated severity
- Two agents report different findings: both preserved
- Agent A says high severity, Agent B says medium for same finding: final is high
- Agent A says APPROVED, Agent B says BLOCKING: final verdict is BLOCKING
- All agents say APPROVED: final verdict is APPROVED
- One agent has error (Err != nil): its findings excluded, counted in stats
- Empty results (zero agents): produces empty ConsolidatedReview with APPROVED verdict
- Single agent: no deduplication needed, findings passed through
- Findings sorted by severity then file then line
- ConsolidationStats overlap rate calculated correctly (duplicates / total)
- Severity escalation: info < low < medium < high < critical
- Three agents report same finding: severity is max of all three

### Integration Tests
- Consolidate realistic multi-agent output with mixed findings and verdicts

### Edge Cases to Handle
- Finding with line=0 (file-level finding) deduplication: key is "file:0:category"
- Same file+line but different categories: treated as distinct findings
- Agent returning zero findings with APPROVED verdict (legitimate, no issues found)
- Agent returning zero findings with BLOCKING verdict (unusual but valid)
- Very large number of findings (1000+): ensure consolidation remains fast
- Finding descriptions with different lengths from different agents

## Implementation Notes
### Recommended Approach
1. Create a `map[string]*Finding` for deduplication, keyed by `Finding.DeduplicationKey()`
2. Create a `map[string][]string` tracking which agents reported each finding
3. Iterate through each AgentReviewResult:
   a. Skip results with non-nil Err (log warning, track in stats)
   b. For each finding, compute dedup key
   c. If key exists in map: escalate severity, append agent name, merge description
   d. If key is new: add to map with agent attribution
4. Collect all verdicts, apply AggregateVerdicts
5. Sort final findings: by severityRank (descending), then file path, then line number
6. Compute stats from the maps

### Potential Pitfalls
- Description merging: avoid creating excessively long merged descriptions when 3+ agents report the same finding. Consider keeping the most detailed description and noting agreement
- Severity escalation should never downgrade (always take the maximum)
- Agent names must be unique across review results -- if two Claude instances run, they need distinct identifiers
- Empty dedup key for findings missing file or category should still work (degenerate but valid)

### Security Considerations
- None specific to this task (pure data transformation of in-memory structs)

## References
- [PRD Section 5.5 - Consolidation: deduplicates findings by file+line+category composite key](docs/prd/PRD-Raven.md)
- [PRD Section 5.5 - Review verdict aggregation](docs/prd/PRD-Raven.md)
