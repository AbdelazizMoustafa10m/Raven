package review

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/charmbracelet/log"
)

//go:embed report_template.tmpl
var defaultReportTemplate string

// ReportGenerator renders a structured markdown code review report from
// consolidated review results.
type ReportGenerator struct {
	tmpl   *template.Template
	logger *log.Logger
}

// reportTemplateData extends ConsolidationStats with sorted keys for
// deterministic template iteration. These are computed once in buildReportData
// and embedded directly into the template data.
type reportTemplateStats struct {
	*ConsolidationStats
	// FindingsPerAgentKeys is the sorted slice of agent names.
	FindingsPerAgentKeys []string
	// FindingsPerSeverityKeys is the sorted slice of severity names (by rank, descending).
	FindingsPerSeverityKeys []Severity
}

// ReportData is the complete data structure passed to the report template.
// All map-derived fields are supplemented with sorted key slices so that
// template iteration is deterministic.
type ReportData struct {
	// Verdict is the aggregated review verdict.
	Verdict Verdict

	// VerdictEmoji is a text indicator: [PASS], [FAIL], or [BLOCK].
	VerdictEmoji string

	// Severity counts for the summary table.
	TotalFindings int
	CriticalCount int
	HighCount     int
	MediumCount   int
	LowCount      int
	InfoCount     int

	// Findings is the sorted slice of all unique findings.
	Findings []*Finding

	// FindingsByFile maps file path to its findings. Use FindingsByFileKeys for
	// deterministic iteration.
	FindingsByFile     map[string][]*Finding
	FindingsByFileKeys []string

	// FindingsBySeverity maps severity to its findings. Use
	// FindingsBySeverityKeys for deterministic iteration.
	FindingsBySeverity     map[Severity][]*Finding
	FindingsBySeverityKeys []Severity

	// AgentResults is the per-agent review result slice.
	AgentResults []AgentReviewResult

	// Stats holds the wrapped consolidation statistics with sorted keys.
	Stats *reportTemplateStats

	// DiffStats summarises the diff.
	DiffStats DiffStats

	// GeneratedAt is the timestamp when the report was generated.
	GeneratedAt time.Time
}

// NewReportGenerator creates a ReportGenerator that uses the embedded default
// report template. logger may be nil.
func NewReportGenerator(logger *log.Logger) *ReportGenerator {
	funcMap := template.FuncMap{
		"escapeCell": escapeCellContent,
		"toUpper":    func(s Severity) string { return strings.ToUpper(string(s)) },
		"agentVerdict": func(ar AgentReviewResult) string {
			if ar.Err != nil {
				return "ERROR"
			}
			if ar.Result == nil {
				return "N/A"
			}
			return string(ar.Result.Verdict)
		},
		"agentFindingCount": func(ar AgentReviewResult) string {
			if ar.Err != nil || ar.Result == nil {
				return "0"
			}
			return fmt.Sprintf("%d", len(ar.Result.Findings))
		},
		"agentStatus": func(ar AgentReviewResult) string {
			if ar.Err != nil {
				return "[FAIL]"
			}
			return "[PASS]"
		},
	}

	tmpl := template.Must(
		template.New("report").
			Delims("[[", "]]").
			Funcs(funcMap).
			Parse(defaultReportTemplate),
	)

	return &ReportGenerator{
		tmpl:   tmpl,
		logger: logger,
	}
}

// Generate renders the markdown report from the given consolidated review,
// consolidation statistics, and diff result. It returns the rendered report
// as a string or an error if template execution fails.
func (rg *ReportGenerator) Generate(
	consolidated *ConsolidatedReview,
	stats *ConsolidationStats,
	diffResult *DiffResult,
) (string, error) {
	if consolidated == nil {
		return "", fmt.Errorf("review: report: consolidated review is required")
	}
	if stats == nil {
		return "", fmt.Errorf("review: report: consolidation stats are required")
	}
	if diffResult == nil {
		return "", fmt.Errorf("review: report: diff result is required")
	}

	data := rg.buildReportData(consolidated, stats, diffResult)

	var buf bytes.Buffer
	if err := rg.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("review: report: executing template: %w", err)
	}

	report := buf.String()

	if rg.logger != nil {
		rg.logger.Info("review report generated",
			"verdict", consolidated.Verdict,
			"findings", len(consolidated.Findings),
			"bytes", len(report),
		)
	}

	return report, nil
}

// WriteToFile generates the report and writes it to the given file path.
// Parent directories are created with 0755 permissions if they do not exist.
// The file is written with 0644 permissions.
func (rg *ReportGenerator) WriteToFile(
	path string,
	consolidated *ConsolidatedReview,
	stats *ConsolidationStats,
	diffResult *DiffResult,
) error {
	report, err := rg.Generate(consolidated, stats, diffResult)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("review: report: creating directory %q: %w", dir, err)
	}

	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		return fmt.Errorf("review: report: writing report to %q: %w", path, err)
	}

	if rg.logger != nil {
		rg.logger.Info("review report written", "path", path)
	}

	return nil
}

// buildReportData transforms the consolidated review, stats, and diff into the
// flat ReportData structure expected by the template. All map keys are sorted
// for deterministic output.
func (rg *ReportGenerator) buildReportData(
	consolidated *ConsolidatedReview,
	stats *ConsolidationStats,
	diffResult *DiffResult,
) ReportData {
	// --- Verdict indicator ---
	verdictEmoji := verdictIndicator(consolidated.Verdict)

	// --- Severity counts ---
	var criticalCount, highCount, mediumCount, lowCount, infoCount int
	for _, f := range consolidated.Findings {
		switch f.Severity {
		case SeverityCritical:
			criticalCount++
		case SeverityHigh:
			highCount++
		case SeverityMedium:
			mediumCount++
		case SeverityLow:
			lowCount++
		case SeverityInfo:
			infoCount++
		}
	}

	// --- Findings by file ---
	findingsByFile := make(map[string][]*Finding)
	for _, f := range consolidated.Findings {
		findingsByFile[f.File] = append(findingsByFile[f.File], f)
	}
	fileKeys := make([]string, 0, len(findingsByFile))
	for k := range findingsByFile {
		fileKeys = append(fileKeys, k)
	}
	sort.Strings(fileKeys)

	// --- Findings by severity ---
	findingsBySeverity := make(map[Severity][]*Finding)
	for _, f := range consolidated.Findings {
		findingsBySeverity[f.Severity] = append(findingsBySeverity[f.Severity], f)
	}
	// Sort severity keys by rank (highest severity first).
	severityKeys := make([]Severity, 0, len(findingsBySeverity))
	for k := range findingsBySeverity {
		severityKeys = append(severityKeys, k)
	}
	sort.Slice(severityKeys, func(i, j int) bool {
		return severityRank(severityKeys[i]) > severityRank(severityKeys[j])
	})

	// --- Stats with sorted keys ---
	agentKeys := make([]string, 0, len(stats.FindingsPerAgent))
	for k := range stats.FindingsPerAgent {
		agentKeys = append(agentKeys, k)
	}
	sort.Strings(agentKeys)

	sevStatKeys := make([]Severity, 0, len(stats.FindingsPerSeverity))
	for k := range stats.FindingsPerSeverity {
		sevStatKeys = append(sevStatKeys, k)
	}
	sort.Slice(sevStatKeys, func(i, j int) bool {
		return severityRank(sevStatKeys[i]) > severityRank(sevStatKeys[j])
	})

	wrappedStats := &reportTemplateStats{
		ConsolidationStats:      stats,
		FindingsPerAgentKeys:    agentKeys,
		FindingsPerSeverityKeys: sevStatKeys,
	}

	// --- DiffStats ---
	var ds DiffStats
	if diffResult != nil {
		ds = diffResult.Stats
	}

	return ReportData{
		Verdict:                consolidated.Verdict,
		VerdictEmoji:           verdictEmoji,
		TotalFindings:          len(consolidated.Findings),
		CriticalCount:          criticalCount,
		HighCount:              highCount,
		MediumCount:            mediumCount,
		LowCount:               lowCount,
		InfoCount:              infoCount,
		Findings:               consolidated.Findings,
		FindingsByFile:         findingsByFile,
		FindingsByFileKeys:     fileKeys,
		FindingsBySeverity:     findingsBySeverity,
		FindingsBySeverityKeys: severityKeys,
		AgentResults:           consolidated.AgentResults,
		Stats:                  wrappedStats,
		DiffStats:              ds,
		GeneratedAt:            time.Now().UTC(),
	}
}

// verdictIndicator returns a text indicator string for a verdict value.
// [PASS] for APPROVED, [FAIL] for CHANGES_NEEDED, [BLOCK] for BLOCKING.
func verdictIndicator(v Verdict) string {
	switch v {
	case VerdictApproved:
		return "[PASS]"
	case VerdictChangesNeeded:
		return "[FAIL]"
	case VerdictBlocking:
		return "[BLOCK]"
	default:
		return "[UNKNOWN]"
	}
}

// escapeCellContent replaces pipe characters and newlines in a string so they
// do not break GitHub Flavored Markdown table cells.
func escapeCellContent(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}
