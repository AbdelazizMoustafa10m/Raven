package review

import (
	"encoding/json"
	"fmt"
	"testing"
)

// BenchmarkJSONExtractReviewResult measures the cost of unmarshalling a
// ReviewResult from a JSON byte slice. This mirrors the hot path taken by
// agent adapters that parse structured agent output.
func BenchmarkJSONExtractReviewResult(b *testing.B) {
	payload := []byte(`{
		"findings": [
			{"severity":"high","category":"security","file":"internal/auth/token.go","line":42,"description":"token stored in plain text","suggestion":"use encrypted storage","agent":"claude"},
			{"severity":"medium","category":"perf","file":"internal/loop/runner.go","line":88,"description":"allocation in hot path","suggestion":"pre-allocate slice","agent":"claude"},
			{"severity":"low","category":"style","file":"main.go","line":5,"description":"missing doc comment","agent":"claude"}
		],
		"verdict":"CHANGES_NEEDED"
	}`)

	b.ResetTimer()
	for b.Loop() {
		var rr ReviewResult
		if err := json.Unmarshal(payload, &rr); err != nil {
			b.Fatalf("json.Unmarshal: %v", err)
		}
	}
}

// buildAgentResults constructs a slice of AgentReviewResult where each agent
// has findingsPerAgent unique findings. Used across multiple benchmarks so the
// data construction cost is always outside b.ResetTimer.
func buildAgentResults(agentNames []string, findingsPerAgent int) []AgentReviewResult {
	severities := []Severity{SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}
	categories := []string{"security", "logic", "perf", "style", "docs"}

	results := make([]AgentReviewResult, len(agentNames))
	for ai, name := range agentNames {
		findings := make([]Finding, findingsPerAgent)
		for i := range findings {
			findings[i] = Finding{
				Severity:    severities[i%len(severities)],
				Category:    categories[i%len(categories)],
				File:        fmt.Sprintf("pkg/module_%d.go", ai),
				Line:        i + 1,
				Description: fmt.Sprintf("issue %d found by %s", i, name),
				Suggestion:  fmt.Sprintf("fix suggestion %d", i),
			}
		}
		results[ai] = AgentReviewResult{
			Agent: name,
			Result: &ReviewResult{
				Findings: findings,
				Verdict:  VerdictChangesNeeded,
			},
		}
	}
	return results
}

// BenchmarkConsolidateFindings measures Consolidate with 3 agents, 10 findings
// each (30 total), no overlap — a small but realistic pipeline payload.
func BenchmarkConsolidateFindings(b *testing.B) {
	agents := []string{"claude", "codex", "gemini"}
	results := buildAgentResults(agents, 10)
	c := NewConsolidator(nil)

	b.ResetTimer()
	for b.Loop() {
		_, _ = c.Consolidate(results)
	}
}

// BenchmarkConsolidateFindingsLarge measures Consolidate with 5 agents, 50
// findings each (250 total), no overlap — stresses the deduplication map and
// the final sort.
func BenchmarkConsolidateFindingsLarge(b *testing.B) {
	agents := []string{"claude", "codex", "gemini", "gpt4", "llama"}
	results := buildAgentResults(agents, 50)
	c := NewConsolidator(nil)

	b.ResetTimer()
	for b.Loop() {
		_, _ = c.Consolidate(results)
	}
}

// BenchmarkFindingDeduplicationKey measures the cost of computing the composite
// deduplication key ("file:line:category") for a single Finding.
func BenchmarkFindingDeduplicationKey(b *testing.B) {
	f := Finding{
		Severity:    SeverityHigh,
		Category:    "security",
		File:        "internal/auth/token.go",
		Line:        42,
		Description: "token stored in plain text",
		Suggestion:  "use encrypted storage",
		Agent:       "claude",
	}

	b.ResetTimer()
	for b.Loop() {
		_ = f.DeduplicationKey()
	}
}
