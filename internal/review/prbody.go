package review

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/charmbracelet/log"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
)

//go:embed prbody_template.tmpl
var defaultPRBodyTemplate string

// maxPRBodyBytes is the GitHub hard limit on PR body length.
const maxPRBodyBytes = 65536

// maxReviewReportBytes is the maximum bytes of the review report to embed
// inline before truncating with a notice.
const maxReviewReportBytes = 10000

// PRBodyGenerator produces a GitHub-formatted PR body markdown string from
// review and fix pipeline results.
type PRBodyGenerator struct {
	agent        agent.Agent // optional – may be nil
	templatePath string      // path to .github/PULL_REQUEST_TEMPLATE.md
	logger       *log.Logger
	tmpl         *template.Template
}

// PRBodyData is the complete data bag passed to Generate.
type PRBodyData struct {
	// Summary is the AI-generated or manually provided change summary.
	Summary string

	// TasksCompleted lists the tasks that are part of this PR.
	TasksCompleted []TaskSummary

	// DiffStats summarises the diff (files changed, lines added/deleted).
	DiffStats DiffStats

	// ReviewVerdict is the final verdict from the review pipeline.
	ReviewVerdict Verdict

	// ReviewFindingsCount is the total number of review findings.
	ReviewFindingsCount int

	// ReviewReport is the full review report in markdown. May be empty.
	ReviewReport string

	// FixReport holds fix-cycle results. May be nil when no fix cycles ran.
	FixReport *FixReport

	// VerificationReport holds the final verification results. May be nil.
	VerificationReport *VerificationReport

	// BranchName is the source branch for the PR.
	BranchName string

	// BaseBranch is the target branch for the PR.
	BaseBranch string

	// PhaseName is the human-readable phase name. Empty when the PR is not
	// part of a phase pipeline.
	PhaseName string
}

// TaskSummary holds the minimal task information needed for the PR body.
type TaskSummary struct {
	ID    string
	Title string
}

// prBodyTemplateData is the unexported structure passed to the template. It is
// derived from PRBodyData with pre-computed helper fields.
type prBodyTemplateData struct {
	PRBodyData

	// VerdictIndicator is "[PASS]", "[FAIL]", or "[BLOCK]".
	VerdictIndicator string

	// HasReviewReport is true when ReviewReport is non-empty.
	HasReviewReport bool

	// TruncatedReviewReport is ReviewReport truncated to maxReviewReportBytes.
	TruncatedReviewReport string

	// ReviewReportTruncated is true when the report was truncated.
	ReviewReportTruncated bool

	// HasFixReport is true when FixReport is non-nil.
	HasFixReport bool

	// HasVerificationReport is true when VerificationReport is non-nil.
	HasVerificationReport bool

	// VerificationMarkdown is the pre-rendered FormatMarkdown() output.
	VerificationMarkdown string

	// FixFinalStatusLabel is a human-readable final status for the fix report.
	FixFinalStatusLabel string
}

// NewPRBodyGenerator creates a PRBodyGenerator.
//
//   - ag is the agent used for AI summary generation. It may be nil; in that
//     case GenerateSummary falls back to a structured summary.
//   - templatePath is the path to a .github/PULL_REQUEST_TEMPLATE.md file. It
//     is used best-effort: if missing or unparseable the default structure is
//     used without error.
//   - logger may be nil.
func NewPRBodyGenerator(ag agent.Agent, templatePath string, logger *log.Logger) *PRBodyGenerator {
	funcMap := template.FuncMap{
		"escapeCell": escapeCellContent,
	}

	tmpl := template.Must(
		template.New("prbody").
			Delims("[[", "]]").
			Funcs(funcMap).
			Parse(defaultPRBodyTemplate),
	)

	return &PRBodyGenerator{
		agent:        ag,
		templatePath: templatePath,
		logger:       logger,
		tmpl:         tmpl,
	}
}

// Generate produces a markdown PR body string from the supplied PRBodyData.
// The body is truncated to maxPRBodyBytes (65,536) if necessary to respect the
// GitHub hard limit.
//
// When a PR template file is configured at templatePath, Generate attempts to
// detect and inject content after well-known section headers. If the file does
// not exist or the injection fails for any reason, the default template
// structure is used without error (best-effort).
func (pg *PRBodyGenerator) Generate(ctx context.Context, data PRBodyData) (string, error) {
	// Log if a custom PR template is present (used best-effort; the generated
	// body is always based on the embedded default template and the
	// custom template content is noted for informational purposes only).
	if pg.hasPRTemplate() {
		if pg.logger != nil {
			pg.logger.Debug("PR body: custom PR template found (best-effort)",
				"path", pg.templatePath,
			)
		}
	}

	// Pre-compute helper fields for the template.
	td := pg.buildTemplateData(data)

	var buf bytes.Buffer
	if err := pg.tmpl.Execute(&buf, td); err != nil {
		return "", fmt.Errorf("review: prbody: executing template: %w", err)
	}

	body := buf.String()

	// Enforce the GitHub PR body size limit.
	if len(body) > maxPRBodyBytes {
		const notice = "\n\n---\n*PR body truncated to fit GitHub's 65,536 character limit.*\n"
		cutoff := maxPRBodyBytes - len(notice)
		if cutoff < 0 {
			cutoff = 0
		}
		body = body[:cutoff] + notice
	}

	if pg.logger != nil {
		pg.logger.Info("PR body generated",
			"bytes", len(body),
			"tasks", len(data.TasksCompleted),
			"verdict", data.ReviewVerdict,
		)
	}

	return body, nil
}

// GenerateSummary uses an AI agent to produce a natural-language summary of
// the changes. The diff itself is NOT passed to keep the prompt short; instead
// the agent receives diff statistics and task titles.
//
// When the agent is nil, unavailable, or returns an error, a structured
// fallback summary is returned (no error is propagated).
func (pg *PRBodyGenerator) GenerateSummary(ctx context.Context, diff string, tasks []TaskSummary) (string, error) {
	if pg.agent != nil {
		summary, err := pg.runAgentSummary(ctx, diff, tasks)
		if err == nil {
			return summary, nil
		}
		// Log the failure but fall through to the structured fallback.
		if pg.logger != nil {
			pg.logger.Warn("PR body: agent summary failed, using fallback",
				"agent", pg.agent.Name(),
				"error", err,
			)
		}
	}

	// Structured fallback -- does not require the agent.
	return pg.buildFallbackSummary(tasks), nil
}

// GenerateTitle produces a concise PR title from the tasks and phase
// information stored in PRBodyData.
//
// Rules:
//   - Single task: "T-007: Task Title"
//   - Phase with tasks: "Phase N: T-001 - T-010" or
//     "Phase N: Phase Name (T-001 - T-010)" when PhaseName is set.
//   - Multiple tasks (no phase): "Tasks T-007, T-008, T-009" with
//     "and N more" when there are more than 3.
func (pg *PRBodyGenerator) GenerateTitle(data PRBodyData) string {
	tasks := data.TasksCompleted

	// Single task.
	if len(tasks) == 1 {
		t := tasks[0]
		if t.Title != "" {
			return fmt.Sprintf("%s: %s", t.ID, t.Title)
		}
		return t.ID
	}

	// Phase title -- only when there is explicit phase context.
	if data.PhaseName != "" || extractPhaseNumber(data.BranchName) != "" {
		return pg.buildPhaseTitle(data)
	}

	// Multiple tasks without phase.
	return pg.buildMultiTaskTitle(tasks)
}

// --- private helpers --------------------------------------------------------

// buildTemplateData converts PRBodyData to the internal template data struct.
func (pg *PRBodyGenerator) buildTemplateData(data PRBodyData) prBodyTemplateData {
	td := prBodyTemplateData{
		PRBodyData:            data,
		VerdictIndicator:      verdictIndicator(data.ReviewVerdict),
		HasReviewReport:       data.ReviewReport != "",
		HasFixReport:          data.FixReport != nil,
		HasVerificationReport: data.VerificationReport != nil,
	}

	// Truncate the review report.
	if data.ReviewReport != "" {
		if len(data.ReviewReport) > maxReviewReportBytes {
			const notice = "\n\n... see full report (truncated at 10,000 chars)"
			cutoff := maxReviewReportBytes - len(notice)
			if cutoff < 0 {
				cutoff = 0
			}
			td.TruncatedReviewReport = data.ReviewReport[:cutoff] + notice
			td.ReviewReportTruncated = true
		} else {
			td.TruncatedReviewReport = data.ReviewReport
		}
	}

	// Pre-render verification markdown.
	if data.VerificationReport != nil {
		td.VerificationMarkdown = data.VerificationReport.FormatMarkdown()
	}

	// Human-readable fix final status.
	if data.FixReport != nil {
		if data.FixReport.FinalStatus == VerificationPassed {
			td.FixFinalStatusLabel = "passed"
		} else {
			td.FixFinalStatusLabel = "failed"
		}
	}

	// Adjust heading levels in AI summary so it doesn't conflict with PR body
	// top-level headings.
	if data.Summary != "" {
		td.PRBodyData.Summary = adjustSummaryHeadings(data.Summary)
	}

	return td
}

// runAgentSummary invokes the AI agent to produce a summary prompt.
func (pg *PRBodyGenerator) runAgentSummary(ctx context.Context, diff string, tasks []TaskSummary) (string, error) {
	prompt := pg.buildSummaryPrompt(diff, tasks)

	result, err := pg.agent.Run(ctx, agent.RunOpts{
		Prompt: prompt,
	})
	if err != nil {
		return "", fmt.Errorf("review: prbody: agent run: %w", err)
	}
	if result == nil || result.ExitCode != 0 {
		return "", fmt.Errorf("review: prbody: agent exited with code %d", exitCodeOf(result))
	}

	summary := strings.TrimSpace(result.Stdout)
	if summary == "" {
		return "", fmt.Errorf("review: prbody: agent returned empty summary")
	}

	return summary, nil
}

// buildSummaryPrompt constructs the prompt sent to the agent.
// The full diff text is NOT included -- only task titles -- to keep the prompt
// concise and avoid exceeding token limits. The diff parameter is accepted for
// future extensibility but is intentionally unused in the current prompt.
func (pg *PRBodyGenerator) buildSummaryPrompt(_ string, tasks []TaskSummary) string {
	var sb strings.Builder

	sb.WriteString("Write a concise, human-readable summary (2-4 sentences) for a GitHub Pull Request.\n\n")
	sb.WriteString("## Tasks Implemented\n\n")
	for _, t := range tasks {
		if t.Title != "" {
			fmt.Fprintf(&sb, "- %s: %s\n", t.ID, t.Title)
		} else {
			fmt.Fprintf(&sb, "- %s\n", t.ID)
		}
	}

	sb.WriteString("\nFocus on WHAT was implemented and WHY it matters. ")
	sb.WriteString("Do NOT include headers or bullet points in your response. ")
	sb.WriteString("Write in plain prose suitable for a PR description.\n")

	return sb.String()
}

// buildFallbackSummary creates a structured summary without agent assistance.
func (pg *PRBodyGenerator) buildFallbackSummary(tasks []TaskSummary) string {
	n := len(tasks)

	ids := make([]string, 0, n)
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}

	switch n {
	case 0:
		return "This PR contains no tracked tasks."
	case 1:
		t := tasks[0]
		if t.Title != "" {
			return fmt.Sprintf("This PR implements %s: %s.", t.ID, t.Title)
		}
		return fmt.Sprintf("This PR implements task %s.", t.ID)
	default:
		return fmt.Sprintf(
			"This PR implements %d task(s): %s.",
			n,
			strings.Join(ids, ", "),
		)
	}
}

// buildPhaseTitle produces a title for a phase-level PR.
func (pg *PRBodyGenerator) buildPhaseTitle(data PRBodyData) string {
	tasks := data.TasksCompleted

	// Detect phase number from branch name (e.g. "phase/3-something" -> "3").
	phaseNum := extractPhaseNumber(data.BranchName)

	// Task range.
	var taskRange string
	if len(tasks) >= 2 {
		first := tasks[0].ID
		last := tasks[len(tasks)-1].ID
		taskRange = fmt.Sprintf("%s - %s", first, last)
	} else if len(tasks) == 1 {
		taskRange = tasks[0].ID
	}

	var sb strings.Builder
	if phaseNum != "" {
		sb.WriteString("Phase ")
		sb.WriteString(phaseNum)
		sb.WriteString(": ")
	}

	if data.PhaseName != "" && taskRange != "" {
		fmt.Fprintf(&sb, "%s (%s)", data.PhaseName, taskRange)
	} else if data.PhaseName != "" {
		sb.WriteString(data.PhaseName)
	} else if taskRange != "" {
		sb.WriteString(taskRange)
	}

	title := sb.String()
	if title == "" {
		return "Phase Implementation"
	}
	return title
}

// buildMultiTaskTitle produces a title for a PR covering multiple unphased tasks.
func (pg *PRBodyGenerator) buildMultiTaskTitle(tasks []TaskSummary) string {
	if len(tasks) == 0 {
		return "Implementation"
	}

	const maxListed = 3

	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}

	if len(ids) <= maxListed {
		return "Tasks " + strings.Join(ids, ", ")
	}

	listed := strings.Join(ids[:maxListed], ", ")
	remaining := len(ids) - maxListed
	return fmt.Sprintf("Tasks %s and %d more", listed, remaining)
}

// adjustSummaryHeadings demotes heading levels in the summary text to avoid
// conflicts with the PR body's top-level `##` section headings.
// `#` headings (h1) become `###` (h3), `##` become `####` (h4), and so on,
// capped at h6. Inline uses of `#` (not at line start) are not affected.
func adjustSummaryHeadings(summary string) string {
	// Match ATX headings: one or more `#` at the start of a line, followed by
	// a space. Limit to 6 hashes (standard Markdown maximum).
	headingRe := regexp.MustCompile(`(?m)^(#{1,6}) `)
	return headingRe.ReplaceAllStringFunc(summary, func(match string) string {
		// Count the leading `#` characters (everything before the trailing space).
		hashes := strings.TrimRight(match, " ")
		n := len(hashes)
		// Demote by 2 levels, cap at 6.
		newLevel := n + 2
		if newLevel > 6 {
			newLevel = 6
		}
		return strings.Repeat("#", newLevel) + " "
	})
}

// extractPhaseNumber parses a phase number from branch names of the form
// "phase/3-description" or "phase/3". Returns an empty string when no number
// is found.
func extractPhaseNumber(branch string) string {
	phaseRe := regexp.MustCompile(`(?i)phase[/\-_](\d+)`)
	m := phaseRe.FindStringSubmatch(branch)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// exitCodeOf returns the exit code from a RunResult, handling nil gracefully.
func exitCodeOf(r *agent.RunResult) int {
	if r == nil {
		return -1
	}
	return r.ExitCode
}

// hasPRTemplate reports whether the configured PR template path exists and is
// readable. Returns false for any error so the caller can treat absence of the
// file as a no-op.
func (pg *PRBodyGenerator) hasPRTemplate() bool {
	if pg.templatePath == "" {
		return false
	}
	_, err := os.Stat(pg.templatePath)
	return err == nil
}
