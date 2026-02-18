package review

import (
	"fmt"
	"time"
)

// Verdict represents the overall outcome of a code review.
type Verdict string

const (
	// VerdictApproved indicates no blocking issues were found.
	VerdictApproved Verdict = "APPROVED"

	// VerdictChangesNeeded indicates non-blocking issues that should be addressed.
	VerdictChangesNeeded Verdict = "CHANGES_NEEDED"

	// VerdictBlocking indicates critical issues that must be resolved before merging.
	VerdictBlocking Verdict = "BLOCKING"
)

// validVerdicts is the set of all known Verdict values.
var validVerdicts = map[Verdict]bool{
	VerdictApproved:      true,
	VerdictChangesNeeded: true,
	VerdictBlocking:      true,
}

// Severity represents the impact level of a review finding.
type Severity string

const (
	// SeverityInfo indicates an informational observation with no required action.
	SeverityInfo Severity = "info"

	// SeverityLow indicates a minor issue that is safe to defer.
	SeverityLow Severity = "low"

	// SeverityMedium indicates a moderate issue that should be addressed.
	SeverityMedium Severity = "medium"

	// SeverityHigh indicates a significant issue that requires attention before merging.
	SeverityHigh Severity = "high"

	// SeverityCritical indicates a showstopper issue that blocks merging.
	SeverityCritical Severity = "critical"
)

// validSeverities is the set of all known Severity values.
var validSeverities = map[Severity]bool{
	SeverityInfo:     true,
	SeverityLow:      true,
	SeverityMedium:   true,
	SeverityHigh:     true,
	SeverityCritical: true,
}

// Finding represents a single issue identified by a reviewing agent.
// The Agent field is empty in raw agent output and is populated during consolidation.
type Finding struct {
	Severity    Severity `json:"severity"`
	Category    string   `json:"category"`
	File        string   `json:"file"`
	Line        int      `json:"line"`
	Description string   `json:"description"`
	Suggestion  string   `json:"suggestion"`
	Agent       string   `json:"agent,omitempty"`
}

// DeduplicationKey returns a composite key of "file:line:category" used to
// identify duplicate findings across multiple agent results during consolidation.
func (f *Finding) DeduplicationKey() string {
	return fmt.Sprintf("%s:%d:%s", f.File, f.Line, f.Category)
}

// ReviewResult holds the parsed output from a single agent's review pass.
type ReviewResult struct {
	Findings []Finding `json:"findings"`
	Verdict  Verdict   `json:"verdict"`
}

// Validate checks that the ReviewResult has valid severity and verdict values.
// It returns nil for a nil receiver. An empty Findings slice with a valid verdict is
// considered valid. Each Finding's Severity must be one of the known Severity constants.
func (rr *ReviewResult) Validate() error {
	if rr == nil {
		return nil
	}
	if !validVerdicts[rr.Verdict] {
		return fmt.Errorf("invalid verdict %q: must be one of APPROVED, CHANGES_NEEDED, BLOCKING", rr.Verdict)
	}
	for i, f := range rr.Findings {
		if !validSeverities[f.Severity] {
			return fmt.Errorf("finding[%d] has invalid severity %q: must be one of info, low, medium, high, critical", i, f.Severity)
		}
	}
	return nil
}

// AgentReviewResult captures the outcome of a single agent's review, including
// timing information and any error that occurred during the review run.
// RawOutput preserves the full agent output for debugging and extraction retries.
type AgentReviewResult struct {
	Agent     string
	Result    *ReviewResult
	Duration  time.Duration
	Err       error
	RawOutput string
}

// ConsolidatedReview is the merged result of all agent review runs for a single
// diff. Findings from all agents are de-duplicated and sorted by severity.
type ConsolidatedReview struct {
	Findings     []*Finding
	Verdict      Verdict
	AgentResults []AgentReviewResult
	TotalAgents  int
	Duration     time.Duration
}

// ReviewConfig holds configuration for the review pipeline, read from the [review]
// section of raven.toml.
type ReviewConfig struct {
	// Extensions is a comma-separated list of file extensions to include in review
	// (e.g. ".go,.ts,.py"). Empty means all extensions are reviewed.
	Extensions string `toml:"extensions"`

	// RiskPatterns is a comma-separated list of glob patterns that mark high-risk files
	// triggering deeper review scrutiny.
	RiskPatterns string `toml:"risk_patterns"`

	// PromptsDir is the directory containing custom review prompt templates.
	PromptsDir string `toml:"prompts_dir"`

	// RulesDir is the directory containing custom review rule definitions.
	RulesDir string `toml:"rules_dir"`

	// ProjectBriefFile is the path to the project brief used to give agents context.
	ProjectBriefFile string `toml:"project_brief_file"`
}

// ReviewMode controls how the diff is distributed across reviewing agents.
type ReviewMode string

const (
	// ReviewModeAll sends the full diff to every agent independently.
	ReviewModeAll ReviewMode = "all"

	// ReviewModeSplit partitions the diff across agents so each agent reviews a
	// non-overlapping subset of changed files.
	ReviewModeSplit ReviewMode = "split"
)

// ReviewOpts specifies runtime options for a review pipeline run.
type ReviewOpts struct {
	// Agents is the ordered list of agent names to invoke (e.g. ["claude", "codex"]).
	Agents []string

	// Concurrency caps the number of agents running simultaneously.
	Concurrency int

	// Mode controls full-diff vs split-diff distribution.
	Mode ReviewMode

	// BaseBranch is the Git ref to diff against (e.g. "main").
	BaseBranch string

	// DryRun prints the review plan without executing any agent.
	DryRun bool
}
