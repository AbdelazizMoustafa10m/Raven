package review

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/charmbracelet/log"

	"github.com/AbdelazizMoustafa10m/Raven/internal/agent"
)

//go:embed fix_template.tmpl
var defaultFixTemplate string

// maxFixDiffBytes is the maximum byte size of a diff included in a fix prompt.
// Diffs larger than this threshold are truncated with a notice.
const maxFixDiffBytes = 50 * 1024 // 50KB

// FixEvent is a structured event emitted during the fix-verify cycle for
// TUI consumption. All fields are populated for every event.
type FixEvent struct {
	// Type is one of: fix_started, cycle_started, agent_invoked,
	// verification_started, verification_result, cycle_completed, fix_completed.
	Type      string
	Cycle     int
	Message   string
	Timestamp time.Time
}

// FixCycleResult captures the outcome of a single fix attempt.
type FixCycleResult struct {
	// Cycle is the 1-based cycle number.
	Cycle int

	// AgentResult is the result returned by the agent. It may be nil if the
	// agent invocation itself returned an error.
	AgentResult *agent.RunResult

	// Verification is the verification report produced after the agent ran.
	// It may be nil when the agent failed and verification was skipped.
	Verification *VerificationReport

	// DiffAfterFix is the output of "git diff" captured after the agent ran.
	// An empty string indicates no changes were detected or git was unavailable.
	DiffAfterFix string

	// Duration is the wall-clock time for the entire cycle.
	Duration time.Duration
}

// FixReport is the aggregate report produced after all fix cycles complete.
type FixReport struct {
	// Cycles holds the ordered results of each fix attempt.
	Cycles []FixCycleResult

	// FinalStatus is the verification status of the last completed cycle, or
	// VerificationPassed when no cycles were needed (zero findings or maxCycles==0).
	FinalStatus VerificationStatus

	// TotalCycles is the number of fix cycles that were attempted.
	TotalCycles int

	// FixesApplied is true when at least one cycle ran and its agent exited
	// with code 0.
	FixesApplied bool

	// Duration is the total wall-clock time for the entire fix run.
	Duration time.Duration
}

// FixOpts specifies runtime options for a fix engine run.
type FixOpts struct {
	// Findings is the list of review findings to fix.
	Findings []*Finding

	// ReviewReport is the full markdown review report, included in the prompt
	// for additional context.
	ReviewReport string

	// BaseBranch is the Git ref the current branch was branched from.
	BaseBranch string

	// DryRun causes Fix to build and return the prompt without invoking the
	// agent or running verification commands.
	DryRun bool

	// MaxCycles overrides the FixEngine's default maximum cycle count when > 0.
	MaxCycles int
}

// fixPromptData is the unexported data structure passed to the fix prompt template.
type fixPromptData struct {
	// Findings is the list of review findings that need to be fixed.
	Findings []*Finding

	// Diff is the unified diff content (possibly truncated).
	Diff string

	// Conventions is the list of project conventions from raven.toml.
	Conventions []string

	// VerifyCommands is the list of verification commands to run after fixing.
	VerifyCommands []string

	// PreviousFailures holds cycle results from prior failed attempts, used to
	// provide context for iterative fixing.
	PreviousFailures []FixCycleResult

	// ReviewReport is the full markdown review report, included in the prompt
	// for additional context about the review findings.
	ReviewReport string

	// BaseBranch is the Git ref the current branch was branched from, giving
	// the agent context about what the changes are being compared against.
	BaseBranch string
}

// FixPromptBuilder constructs fix prompts from review findings, diffs, and
// project conventions.
type FixPromptBuilder struct {
	conventions    []string
	verifyCommands []string
	logger         *log.Logger
	tmpl           *template.Template
}

// NewFixPromptBuilder creates a FixPromptBuilder with the given conventions and
// verification commands. Both slices are defensively copied at construction
// time. logger may be nil.
func NewFixPromptBuilder(
	conventions []string,
	verifyCommands []string,
	logger *log.Logger,
) *FixPromptBuilder {
	// Defensive copies so the caller cannot mutate the slices after construction.
	convsCopy := make([]string, len(conventions))
	copy(convsCopy, conventions)

	cmdsCopy := make([]string, len(verifyCommands))
	copy(cmdsCopy, verifyCommands)

	tmpl := template.Must(
		template.New("fix").
			Delims("[[", "]]").
			Parse(defaultFixTemplate),
	)

	return &FixPromptBuilder{
		conventions:    convsCopy,
		verifyCommands: cmdsCopy,
		logger:         logger,
		tmpl:           tmpl,
	}
}

// BuildOpts holds optional context for fix prompt construction.
type BuildOpts struct {
	// ReviewReport is the full markdown review report to include in the prompt.
	ReviewReport string

	// BaseBranch is the Git ref the current branch was branched from.
	BaseBranch string
}

// Build constructs a fix prompt from the provided findings, diff content, and
// the results of any prior failed fix cycles. The diff is truncated to
// maxFixDiffBytes when it exceeds the limit. Additional context (review report,
// base branch) can be supplied via buildOpts; a nil value is safe.
func (fpb *FixPromptBuilder) Build(
	findings []*Finding,
	diff string,
	previousFailures []FixCycleResult,
	buildOpts ...BuildOpts,
) (string, error) {
	// Truncate large diffs.
	if len(diff) > maxFixDiffBytes {
		diff = diff[:maxFixDiffBytes] + "\n... [diff truncated at 50KB] ..."
	}

	data := fixPromptData{
		Findings:         findings,
		Diff:             diff,
		Conventions:      fpb.conventions,
		VerifyCommands:   fpb.verifyCommands,
		PreviousFailures: previousFailures,
	}

	// Apply optional context from BuildOpts.
	if len(buildOpts) > 0 {
		data.ReviewReport = buildOpts[0].ReviewReport
		data.BaseBranch = buildOpts[0].BaseBranch
	}

	var buf bytes.Buffer
	if err := fpb.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("review: fix: executing fix prompt template: %w", err)
	}

	return buf.String(), nil
}

// FixEngine orchestrates the iterative fix-verify cycle. It invokes an AI
// agent to apply review findings, then runs verification commands to confirm
// the fixes are correct. If verification fails and cycles remain, the process
// repeats with failure context appended to the prompt.
type FixEngine struct {
	agent         agent.Agent
	verifier      *VerificationRunner
	promptBuilder *FixPromptBuilder
	maxCycles     int
	logger        *log.Logger
	events        chan<- FixEvent
}

// NewFixEngine creates a FixEngine with the given dependencies. maxCycles
// controls the upper bound on fix attempts; a value <= 0 means no fixes will
// be applied. events may be nil — events are dropped when the channel is nil
// or full.
func NewFixEngine(
	ag agent.Agent,
	verifier *VerificationRunner,
	maxCycles int,
	logger *log.Logger,
	events chan<- FixEvent,
) *FixEngine {
	return &FixEngine{
		agent:         ag,
		verifier:      verifier,
		promptBuilder: nil, // lazily assigned; caller may set via setter or Build creates one
		maxCycles:     maxCycles,
		logger:        logger,
		events:        events,
	}
}

// WithPromptBuilder replaces the FixEngine's prompt builder. This is useful
// when the caller needs custom conventions or verification commands.
func (fe *FixEngine) WithPromptBuilder(pb *FixPromptBuilder) *FixEngine {
	fe.promptBuilder = pb
	return fe
}

// ensurePromptBuilder initialises a default (empty) FixPromptBuilder when none
// has been assigned.
func (fe *FixEngine) ensurePromptBuilder() {
	if fe.promptBuilder == nil {
		fe.promptBuilder = NewFixPromptBuilder(nil, nil, fe.logger)
	}
}

// Fix runs the fix-verify cycle up to maxCycles times (or opts.MaxCycles when
// > 0). For each cycle it:
//  1. Builds a fix prompt from the findings.
//  2. Invokes the agent.
//  3. Captures git diff output after the agent runs.
//  4. Runs verification commands.
//  5. Breaks if verification passes; otherwise repeats with failure context.
//
// Context cancellation returns whatever partial results have been collected so
// far (no error is returned — the FixReport.TotalCycles reflects actual
// progress).
//
// Agent errors (non-zero exit code) do NOT abort the loop; the cycle is
// recorded and the loop continues.
func (fe *FixEngine) Fix(ctx context.Context, opts FixOpts) (*FixReport, error) {
	fe.ensurePromptBuilder()

	start := time.Now()

	maxCycles := fe.maxCycles
	if opts.MaxCycles > 0 {
		maxCycles = opts.MaxCycles
	}

	// When findings are empty but a review report is provided, attempt to
	// extract findings from the report content so the fix engine actually
	// has work to do (the FixHandler in workflow/handlers.go may pass empty
	// findings with a review_report_path).
	if len(opts.Findings) == 0 && opts.ReviewReport != "" {
		extracted := extractFindingsFromReport(opts.ReviewReport)
		if len(extracted) > 0 {
			opts.Findings = extracted
			if fe.logger != nil {
				fe.logger.Info("extracted findings from review report",
					"count", len(extracted),
				)
			}
		}
	}

	// Fast paths: nothing to do.
	if maxCycles <= 0 || len(opts.Findings) == 0 {
		return &FixReport{
			FinalStatus: VerificationPassed,
			Duration:    time.Since(start),
		}, nil
	}

	fe.emit(FixEvent{
		Type:      "fix_started",
		Message:   fmt.Sprintf("starting fix with %d finding(s), max %d cycle(s)", len(opts.Findings), maxCycles),
		Timestamp: time.Now(),
	})

	if fe.logger != nil {
		fe.logger.Info("fix engine started",
			"findings", len(opts.Findings),
			"max_cycles", maxCycles,
		)
	}

	cycles := make([]FixCycleResult, 0, maxCycles)
	fixesApplied := false
	finalStatus := VerificationFailed

	for cycle := 1; cycle <= maxCycles; cycle++ {
		// Honour context cancellation between cycles.
		if err := ctx.Err(); err != nil {
			break
		}

		cycleStart := time.Now()

		fe.emit(FixEvent{
			Type:      "cycle_started",
			Cycle:     cycle,
			Message:   fmt.Sprintf("starting fix cycle %d of %d", cycle, maxCycles),
			Timestamp: time.Now(),
		})

		if fe.logger != nil {
			fe.logger.Info("fix cycle started", "cycle", cycle, "max_cycles", maxCycles)
		}

		// Build the fix prompt, including previous failures and review context.
		currentDiff := captureGitDiff()
		prompt, err := fe.promptBuilder.Build(opts.Findings, currentDiff, cycles, BuildOpts{
			ReviewReport: opts.ReviewReport,
			BaseBranch:   opts.BaseBranch,
		})
		if err != nil {
			return nil, fmt.Errorf("review: fix: building prompt for cycle %d: %w", cycle, err)
		}

		// Invoke the agent.
		fe.emit(FixEvent{
			Type:      "agent_invoked",
			Cycle:     cycle,
			Message:   fmt.Sprintf("invoking agent %s for cycle %d", fe.agent.Name(), cycle),
			Timestamp: time.Now(),
		})

		agentResult, agentErr := fe.agent.Run(ctx, agent.RunOpts{
			Prompt: prompt,
		})
		if agentErr != nil {
			if fe.logger != nil {
				fe.logger.Warn("agent run error during fix cycle",
					"cycle", cycle,
					"agent", fe.agent.Name(),
					"error", agentErr,
				)
			}
			// Record the cycle with a nil AgentResult and nil Verification.
			cycles = append(cycles, FixCycleResult{
				Cycle:        cycle,
				AgentResult:  nil,
				Verification: nil,
				DiffAfterFix: "",
				Duration:     time.Since(cycleStart),
			})

			fe.emit(FixEvent{
				Type:      "cycle_completed",
				Cycle:     cycle,
				Message:   fmt.Sprintf("cycle %d completed with agent error: %v", cycle, agentErr),
				Timestamp: time.Now(),
			})
			continue
		}

		// Track whether any cycle produced a successful agent result.
		if agentResult.ExitCode == 0 {
			fixesApplied = true
		}

		// Capture what the agent changed.
		diffAfterFix := captureGitDiff()

		// Run verification.
		fe.emit(FixEvent{
			Type:      "verification_started",
			Cycle:     cycle,
			Message:   fmt.Sprintf("running verification after cycle %d", cycle),
			Timestamp: time.Now(),
		})

		var verReport *VerificationReport
		if fe.verifier != nil {
			vr, verErr := fe.verifier.Run(ctx, false)
			if verErr != nil {
				// Context was cancelled during verification.
				cycles = append(cycles, FixCycleResult{
					Cycle:        cycle,
					AgentResult:  agentResult,
					Verification: vr,
					DiffAfterFix: diffAfterFix,
					Duration:     time.Since(cycleStart),
				})
				fe.emit(FixEvent{
					Type:      "cycle_completed",
					Cycle:     cycle,
					Message:   fmt.Sprintf("cycle %d verification interrupted: %v", cycle, verErr),
					Timestamp: time.Now(),
				})
				break
			}
			verReport = vr
		}

		fe.emit(FixEvent{
			Type:      "verification_result",
			Cycle:     cycle,
			Message:   verificationResultMessage(verReport),
			Timestamp: time.Now(),
		})

		if fe.logger != nil && verReport != nil {
			fe.logger.Info("fix cycle verification",
				"cycle", cycle,
				"status", verReport.Status,
				"passed", verReport.Passed,
				"failed", verReport.Failed,
			)
		}

		cycleResult := FixCycleResult{
			Cycle:        cycle,
			AgentResult:  agentResult,
			Verification: verReport,
			DiffAfterFix: diffAfterFix,
			Duration:     time.Since(cycleStart),
		}
		cycles = append(cycles, cycleResult)

		fe.emit(FixEvent{
			Type:      "cycle_completed",
			Cycle:     cycle,
			Message:   fmt.Sprintf("cycle %d completed", cycle),
			Timestamp: time.Now(),
		})

		// Update final status from the last completed verification.
		if verReport != nil {
			finalStatus = verReport.Status
		}

		// If verification passed, we are done.
		if verReport != nil && verReport.Status == VerificationPassed {
			break
		}
	}

	report := &FixReport{
		Cycles:       cycles,
		FinalStatus:  finalStatus,
		TotalCycles:  len(cycles),
		FixesApplied: fixesApplied,
		Duration:     time.Since(start),
	}

	fe.emit(FixEvent{
		Type:      "fix_completed",
		Message:   fmt.Sprintf("fix completed: %d cycle(s), status %s", len(cycles), finalStatus),
		Timestamp: time.Now(),
	})

	if fe.logger != nil {
		fe.logger.Info("fix engine completed",
			"total_cycles", len(cycles),
			"fixes_applied", fixesApplied,
			"final_status", finalStatus,
			"duration", time.Since(start),
		)
	}

	return report, nil
}

// DryRun builds and returns the fix prompt without invoking the agent or
// running any verification commands. It emits a fix_started event.
func (fe *FixEngine) DryRun(ctx context.Context, opts FixOpts) (string, error) {
	fe.ensurePromptBuilder()

	fe.emit(FixEvent{
		Type:      "fix_started",
		Message:   fmt.Sprintf("dry run: %d finding(s)", len(opts.Findings)),
		Timestamp: time.Now(),
	})

	if len(opts.Findings) == 0 {
		return "", nil
	}

	diff := captureGitDiff()
	prompt, err := fe.promptBuilder.Build(opts.Findings, diff, nil, BuildOpts{
		ReviewReport: opts.ReviewReport,
		BaseBranch:   opts.BaseBranch,
	})
	if err != nil {
		return "", fmt.Errorf("review: fix: dry run: building prompt: %w", err)
	}

	return prompt, nil
}

// emit sends a FixEvent to the events channel using a non-blocking send.
// If the channel is nil or full the event is silently dropped.
func (fe *FixEngine) emit(ev FixEvent) {
	if fe.events == nil {
		return
	}
	select {
	case fe.events <- ev:
	default:
		// Drop the event rather than blocking.
	}
}

// extractFindingsFromReport attempts to parse review findings from a markdown
// report. It first looks for an embedded JSON block containing a "findings"
// array, then falls back to parsing markdown finding headers of the form
// "### Finding: file.go:42 (high)". Returns nil when no findings are found.
func extractFindingsFromReport(report string) []*Finding {
	// Strategy 1: Look for an embedded JSON block with findings.
	if findings := extractFindingsFromJSON(report); len(findings) > 0 {
		return findings
	}

	// Strategy 2: Parse markdown finding sections produced by the report template.
	return extractFindingsFromMarkdown(report)
}

// extractFindingsFromJSON scans for JSON code blocks in the report that contain
// a "findings" array and attempts to unmarshal them.
func extractFindingsFromJSON(report string) []*Finding {
	// Match fenced JSON code blocks.
	jsonBlockRe := regexp.MustCompile("(?s)```json\\s*\\n(.*?)\\n```")
	matches := jsonBlockRe.FindAllStringSubmatch(report, -1)

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		block := strings.TrimSpace(m[1])
		if !strings.Contains(block, "findings") {
			continue
		}

		// Try parsing as a ReviewResult (has findings + verdict).
		var rr ReviewResult
		if err := json.Unmarshal([]byte(block), &rr); err == nil && len(rr.Findings) > 0 {
			findings := make([]*Finding, len(rr.Findings))
			for i := range rr.Findings {
				f := rr.Findings[i]
				findings[i] = &f
			}
			return findings
		}

		// Try parsing as a bare findings array wrapper.
		var wrapper struct {
			Findings []Finding `json:"findings"`
		}
		if err := json.Unmarshal([]byte(block), &wrapper); err == nil && len(wrapper.Findings) > 0 {
			findings := make([]*Finding, len(wrapper.Findings))
			for i := range wrapper.Findings {
				f := wrapper.Findings[i]
				findings[i] = &f
			}
			return findings
		}
	}

	return nil
}

// extractFindingsFromMarkdown parses structured finding sections from the
// markdown report template output. It looks for table rows with severity,
// category, file, line, and description columns, as well as structured
// headers like "### file.go:42 (high)".
func extractFindingsFromMarkdown(report string) []*Finding {
	var findings []*Finding

	// Pattern: lines that look like markdown table rows with finding data.
	// The report template produces rows like:
	// "| high | security | main.go | 42 | SQL injection risk | claude |"
	// We capture severity, category, file, line, and description. The trailing
	// column(s) are agent attribution, which we skip.
	tableRowRe := regexp.MustCompile(`(?m)^\|\s*(critical|high|medium|low|info)\s*\|\s*([^|]+?)\s*\|\s*([^|]+?)\s*\|\s*(\d+)\s*\|\s*([^|]+?)\s*\|`)
	tableMatches := tableRowRe.FindAllStringSubmatch(report, -1)
	for _, m := range tableMatches {
		if len(m) < 6 {
			continue
		}
		line := 0
		if _, err := fmt.Sscanf(m[4], "%d", &line); err != nil {
			continue
		}
		findings = append(findings, &Finding{
			Severity:    Severity(strings.TrimSpace(m[1])),
			Category:    strings.TrimSpace(m[2]),
			File:        strings.TrimSpace(m[3]),
			Line:        line,
			Description: strings.TrimSpace(m[5]),
		})
	}

	if len(findings) > 0 {
		return findings
	}

	// Fallback: parse "### file.go:42 (severity)" style headers from the
	// report's "Findings by File" section.
	headerRe := regexp.MustCompile(`(?m)^###?\s+` + "`?" + `([^:` + "`" + `]+):(\d+)` + "`?" + `\s*\((\w+)\)`)
	headerMatches := headerRe.FindAllStringSubmatch(report, -1)
	for _, m := range headerMatches {
		if len(m) < 4 {
			continue
		}
		line := 0
		if _, err := fmt.Sscanf(m[2], "%d", &line); err != nil {
			continue
		}
		sev := Severity(strings.ToLower(strings.TrimSpace(m[3])))
		if !validSeverities[sev] {
			sev = SeverityMedium
		}
		findings = append(findings, &Finding{
			Severity:    sev,
			File:        strings.TrimSpace(m[1]),
			Line:        line,
			Category:    "review",
			Description: "Finding from review report.",
		})
	}

	return findings
}

// captureGitDiff runs "git diff" in the current working directory and returns
// the output. If the command fails for any reason, an empty string is returned.
func captureGitDiff() string {
	cmd := exec.Command("git", "diff")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// verificationResultMessage constructs a human-readable message summarising a
// VerificationReport. It handles a nil report gracefully.
func verificationResultMessage(vr *VerificationReport) string {
	if vr == nil {
		return "verification skipped (no verifier configured)"
	}
	return fmt.Sprintf("verification %s: %d/%d passed", vr.Status, vr.Passed, vr.Total)
}
