package review

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/log"
)

// Consolidator merges findings from multiple agent reviews into a single
// deduplicated, severity-escalated result with an aggregated verdict.
type Consolidator struct {
	logger *log.Logger
}

// ConsolidationStats captures metrics about the consolidation process,
// including how many findings were merged and how the overlap rate compares
// across agents.
type ConsolidationStats struct {
	// TotalInputFindings is the sum of all findings across all agent results
	// that did not return an error.
	TotalInputFindings int

	// UniqueFindings is the number of distinct findings after deduplication.
	UniqueFindings int

	// DuplicatesRemoved is the number of redundant findings that were merged
	// into an existing finding.
	DuplicatesRemoved int

	// SeverityEscalations is the number of times a duplicate finding caused
	// the stored severity to be promoted to a higher level.
	SeverityEscalations int

	// OverlapRate is the percentage (0-100) of unique findings that were
	// reported by two or more agents.
	OverlapRate float64

	// FindingsPerAgent maps agent name to the number of findings it contributed
	// (before deduplication).
	FindingsPerAgent map[string]int

	// FindingsPerSeverity maps severity level to the number of unique findings
	// at that level in the consolidated output.
	FindingsPerSeverity map[Severity]int
}

// NewConsolidator creates a Consolidator. logger may be nil; when non-nil it
// receives structured log output for skipped agents and severity escalations.
func NewConsolidator(logger *log.Logger) *Consolidator {
	return &Consolidator{logger: logger}
}

// Consolidate merges findings from multiple agent reviews into a single
// deduplicated, severity-escalated result with an aggregated verdict.
//
// Results with a non-nil Err are excluded from finding aggregation; their
// effective verdict is treated as CHANGES_NEEDED.
//
// The returned ConsolidatedReview has its Findings sorted by severity
// (critical first), then by file path, then by line number.
func (c *Consolidator) Consolidate(results []AgentReviewResult) (*ConsolidatedReview, *ConsolidationStats) {
	stats := &ConsolidationStats{
		FindingsPerAgent:    make(map[string]int),
		FindingsPerSeverity: make(map[Severity]int),
	}

	if len(results) == 0 {
		return &ConsolidatedReview{
			Verdict:      VerdictApproved,
			AgentResults: results,
			TotalAgents:  0,
		}, stats
	}

	// findingMap stores the canonical (deduplicated) finding per composite key.
	findingMap := make(map[string]*Finding)

	// agentsByKey tracks which agents reported each finding key.
	agentsByKey := make(map[string][]string)

	// verdicts collects per-agent verdicts for later aggregation.
	verdicts := make([]Verdict, 0, len(results))

	for _, ar := range results {
		if ar.Err != nil {
			// Log the failure and treat it as CHANGES_NEEDED for verdict purposes.
			if c.logger != nil {
				c.logger.Warn("skipping agent findings due to error",
					"agent", ar.Agent,
					"error", ar.Err,
				)
			}
			verdicts = append(verdicts, VerdictChangesNeeded)
			continue
		}

		if ar.Result == nil {
			// No result struct at all; treat same as error.
			if c.logger != nil {
				c.logger.Warn("skipping agent: nil result",
					"agent", ar.Agent,
				)
			}
			verdicts = append(verdicts, VerdictChangesNeeded)
			continue
		}

		verdicts = append(verdicts, ar.Result.Verdict)

		for i := range ar.Result.Findings {
			f := &ar.Result.Findings[i]

			stats.TotalInputFindings++
			stats.FindingsPerAgent[ar.Agent]++

			key := f.DeduplicationKey()

			existing, seen := findingMap[key]
			if !seen {
				// First time we see this finding: store a copy with agent attribution.
				copied := *f
				copied.Agent = ar.Agent
				findingMap[key] = &copied
				agentsByKey[key] = []string{ar.Agent}
				continue
			}

			// Duplicate: escalate severity, merge description, track agents.
			stats.DuplicatesRemoved++

			escalated := EscalateSeverity(existing.Severity, f.Severity)
			if escalated != existing.Severity {
				if c.logger != nil {
					c.logger.Debug("severity escalated",
						"key", key,
						"from", existing.Severity,
						"to", escalated,
						"agent", ar.Agent,
					)
				}
				existing.Severity = escalated
				stats.SeverityEscalations++
			}

			// Merge description: keep the most detailed (longest) description and
			// note agreement from additional agents, capped to avoid bloat.
			existing.Description = mergeDescriptions(existing.Description, f.Description)

			// Track which agents reported this finding.
			agentsByKey[key] = append(agentsByKey[key], ar.Agent)
		}
	}

	// Build final agent attribution and collect unique findings.
	findings := make([]*Finding, 0, len(findingMap))
	multiAgentCount := 0

	for key, f := range findingMap {
		agents := agentsByKey[key]
		f.Agent = strings.Join(agents, ", ")
		if len(agents) > 1 {
			multiAgentCount++
		}
		findings = append(findings, f)
		stats.FindingsPerSeverity[f.Severity]++
	}

	stats.UniqueFindings = len(findings)

	if stats.UniqueFindings > 0 {
		stats.OverlapRate = float64(multiAgentCount) / float64(stats.UniqueFindings) * 100
	}

	// Sort: critical first (descending severity rank), then file path, then line.
	sort.Slice(findings, func(i, j int) bool {
		ri := severityRank(findings[i].Severity)
		rj := severityRank(findings[j].Severity)
		if ri != rj {
			// Higher rank = higher severity = comes first.
			return ri > rj
		}
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})

	consolidated := &ConsolidatedReview{
		Findings:     findings,
		Verdict:      AggregateVerdicts(verdicts),
		AgentResults: results,
		TotalAgents:  len(results),
	}

	return consolidated, stats
}

// AggregateVerdicts computes the final verdict from per-agent verdicts using
// the rule: BLOCKING > CHANGES_NEEDED > APPROVED.
// An empty input returns APPROVED.
func AggregateVerdicts(verdicts []Verdict) Verdict {
	result := VerdictApproved
	for _, v := range verdicts {
		switch v {
		case VerdictBlocking:
			// Highest possible — short-circuit immediately.
			return VerdictBlocking
		case VerdictChangesNeeded:
			result = VerdictChangesNeeded
		}
	}
	return result
}

// EscalateSeverity returns the higher of two severity levels.
// It never downgrades: if a is already higher than b, a is returned unchanged.
func EscalateSeverity(a, b Severity) Severity {
	if severityRank(b) > severityRank(a) {
		return b
	}
	return a
}

// severityRank returns a numeric rank for severity comparison.
// Higher numbers indicate higher (more severe) levels.
func severityRank(s Severity) int {
	switch s {
	case SeverityInfo:
		return 1
	case SeverityLow:
		return 2
	case SeverityMedium:
		return 3
	case SeverityHigh:
		return 4
	case SeverityCritical:
		return 5
	default:
		return 0
	}
}

// mergeDescriptions combines two finding descriptions into a single string.
// It keeps the longer (more detailed) description as the primary and appends a
// short "Additional note:" when the secondary description adds unique content.
// The note is truncated to maxSecondaryNote bytes to prevent unbounded growth
// when 3+ agents each contribute unique wording.
//
// Agent attribution is tracked separately via agentsByKey and is not included
// in the merged description.
func mergeDescriptions(primary, secondary string) string {
	primary = strings.TrimSpace(primary)
	secondary = strings.TrimSpace(secondary)

	if secondary == "" || primary == secondary {
		return primary
	}

	// Prefer the longer description as the primary.
	if len(secondary) > len(primary) {
		primary, secondary = secondary, primary
	}

	// Check whether the secondary content is already present in primary to avoid
	// redundant appending when two agents produce nearly identical descriptions.
	if strings.Contains(primary, secondary) {
		return primary
	}

	// Append a concise note. We truncate the secondary to avoid runaway lengths
	// when 3+ agents each contribute unique wording.
	const maxSecondaryNote = 120
	note := secondary
	if len(note) > maxSecondaryNote {
		note = fmt.Sprintf("%s...", note[:maxSecondaryNote])
	}

	return fmt.Sprintf("%s\nAdditional note: %s", primary, note)
}
