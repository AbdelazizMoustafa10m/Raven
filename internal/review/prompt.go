package review

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/charmbracelet/log"
)

//go:embed review_template.tmpl
var defaultReviewTemplate string

// maxDiffBytes is the maximum number of bytes of diff content to include in a
// prompt before truncation. Diffs larger than this are truncated with a note.
const maxDiffBytes = 100 * 1024 // 100KB

// maxFilesInList is the maximum number of files to list in the prompt before
// truncating with a count note.
const maxFilesInList = 500

// jsonSchemaExample is the canonical JSON schema example included in every
// review prompt to guide the agent's output format.
const jsonSchemaExample = `{
  "findings": [
    {
      "severity": "high|low|medium|critical|info",
      "category": "security|performance|correctness|style|...",
      "file": "path/to/file.go",
      "line": 42,
      "description": "Description of the issue",
      "suggestion": "How to fix it"
    }
  ],
  "verdict": "APPROVED|CHANGES_NEEDED|BLOCKING"
}`

// PromptData holds all the data needed to render a review prompt template.
type PromptData struct {
	// ProjectBrief is the full text of the project brief file, if available.
	ProjectBrief string

	// Rules contains the text content of each rule file, in sorted order.
	Rules []string

	// Diff is the full unified diff text, possibly truncated.
	Diff string

	// Files is the list of changed files to review (all or a split subset).
	Files []ChangedFile

	// FileList is a pre-formatted multi-line string listing files with risk
	// annotations and change statistics.
	FileList string

	// HighRiskFiles holds the paths of files classified as RiskHigh.
	HighRiskFiles []string

	// Stats is the aggregate diff summary.
	Stats DiffStats

	// JSONSchema is the JSON schema example string for the agent's output.
	JSONSchema string

	// AgentName is the name of the reviewing agent (e.g. "claude").
	AgentName string

	// ReviewMode is the mode the review is running in ("all" or "split").
	ReviewMode ReviewMode
}

// ProjectContext holds the loaded project brief and review rules.
type ProjectContext struct {
	// Brief is the full text of the project brief file.
	Brief string

	// Rules contains the text content of each .md rule file, in sorted order.
	Rules []string
}

// ContextLoader reads the project brief and review rule files from disk.
type ContextLoader struct {
	briefPath string
	rulesDir  string
}

// NewContextLoader creates a ContextLoader that reads from briefPath and
// rulesDir. Paths are cleaned but not validated here; validation happens in
// Load when files are actually accessed.
func NewContextLoader(briefPath, rulesDir string) *ContextLoader {
	return &ContextLoader{
		briefPath: briefPath,
		rulesDir:  rulesDir,
	}
}

// Load reads the project brief and all *.md rule files. Missing files and
// directories are silently skipped; only I/O errors on existing paths are
// returned. Returns an empty ProjectContext when neither path is set.
func (cl *ContextLoader) Load() (*ProjectContext, error) {
	ctx := &ProjectContext{}

	if cl.briefPath != "" {
		if err := validatePath(cl.briefPath); err != nil {
			return nil, fmt.Errorf("review: prompt: project brief path: %w", err)
		}
		data, err := os.ReadFile(cl.briefPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("review: prompt: reading project brief %q: %w", cl.briefPath, err)
			}
			// Missing brief is not an error.
		} else {
			ctx.Brief = string(data)
		}
	}

	if cl.rulesDir != "" {
		if err := validatePath(cl.rulesDir); err != nil {
			return nil, fmt.Errorf("review: prompt: rules dir path: %w", err)
		}
		rules, err := loadRuleFiles(cl.rulesDir)
		if err != nil {
			return nil, err
		}
		ctx.Rules = rules
	}

	return ctx, nil
}

// loadRuleFiles reads all *.md files from dir in alphabetical order. If dir
// does not exist it returns nil, nil. Non-markdown files are skipped.
func loadRuleFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("review: prompt: reading rules dir %q: %w", dir, err)
	}

	// Collect .md file names sorted alphabetically (ReadDir already returns
	// entries in directory order, which is typically alphabetical on most
	// systems, but we sort explicitly for determinism).
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	rules := make([]string, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("review: prompt: reading rule file %q: %w", path, err)
		}
		rules = append(rules, string(data))
	}
	return rules, nil
}

// validatePath rejects paths containing ".." segments after cleaning, which
// could be used for directory traversal.
func validatePath(p string) error {
	cleaned := filepath.Clean(p)
	// filepath.Clean collapses ".." sequences; if the cleaned path still
	// contains ".." as a component, the input was trying to escape the root.
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return fmt.Errorf("path traversal rejected: %q", p)
		}
	}
	return nil
}

// PromptBuilder constructs review prompts from templates and project context.
type PromptBuilder struct {
	cfg    ReviewConfig
	loader *ContextLoader
	logger *log.Logger
}

// NewPromptBuilder creates a PromptBuilder configured from cfg. A ContextLoader
// is initialised from cfg.ProjectBriefFile and cfg.RulesDir. logger may be nil.
func NewPromptBuilder(cfg ReviewConfig, logger *log.Logger) *PromptBuilder {
	return &PromptBuilder{
		cfg:    cfg,
		loader: NewContextLoader(cfg.ProjectBriefFile, cfg.RulesDir),
		logger: logger,
	}
}

// Build renders the review prompt template with the supplied data. It loads
// the custom template from cfg.PromptsDir when available, falling back to the
// embedded default. ctx is accepted for future cancellation support.
func (pb *PromptBuilder) Build(_ context.Context, data PromptData) (string, error) {
	tmplStr, tmplPath, err := pb.loadTemplateText()
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("review").Delims("[[", "]]").Parse(tmplStr)
	if err != nil {
		if tmplPath != "" {
			return "", fmt.Errorf("review: prompt: parse template %q: %w", tmplPath, err)
		}
		return "", fmt.Errorf("review: prompt: parse embedded template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("review: prompt: execute template: %w", err)
	}
	return buf.String(), nil
}

// BuildForAgent constructs a complete prompt for a named agent reviewing a
// specific set of files from a DiffResult. It loads project context, formats
// the file list, and delegates to Build.
func (pb *PromptBuilder) BuildForAgent(
	ctx context.Context,
	agentName string,
	diff *DiffResult,
	files []ChangedFile,
	mode ReviewMode,
) (string, error) {
	projectCtx, err := pb.loader.Load()
	if err != nil {
		return "", fmt.Errorf("review: prompt: loading project context: %w", err)
	}

	if pb.logger != nil {
		pb.logger.Debug("building review prompt",
			"agent", agentName,
			"files", len(files),
			"mode", mode,
			"has_brief", projectCtx.Brief != "",
			"rules", len(projectCtx.Rules),
		)
	}

	// Determine diff content and stats based on the files assigned to this agent.
	fullDiff := diff.FullDiff
	stats := diff.Stats

	// In split mode the Stats in the DiffResult reflect the whole diff, but
	// the prompt should show stats for the agent's file subset only.
	if mode == ReviewModeSplit {
		stats = computeStats(files)
	}

	// Truncate very large diffs.
	if len(fullDiff) > maxDiffBytes {
		fullDiff = fullDiff[:maxDiffBytes] + "\n... [diff truncated at 100KB] ..."
	}

	// Build file list string and extract high-risk paths.
	fileList, highRiskFiles := formatFileList(files)

	data := PromptData{
		ProjectBrief:  projectCtx.Brief,
		Rules:         projectCtx.Rules,
		Diff:          fullDiff,
		Files:         files,
		FileList:      fileList,
		HighRiskFiles: highRiskFiles,
		Stats:         stats,
		JSONSchema:    jsonSchemaExample,
		AgentName:     agentName,
		ReviewMode:    mode,
	}

	return pb.Build(ctx, data)
}

// loadTemplateText attempts to load a custom template from cfg.PromptsDir.
// It checks for "review.tmpl" then "review.md". If neither exists, or
// PromptsDir is empty, the embedded default template is returned.
// The second return value is the path of the loaded file (empty for embedded).
func (pb *PromptBuilder) loadTemplateText() (string, string, error) {
	if pb.cfg.PromptsDir == "" {
		return defaultReviewTemplate, "", nil
	}

	if err := validatePath(pb.cfg.PromptsDir); err != nil {
		return "", "", fmt.Errorf("review: prompt: prompts dir path: %w", err)
	}

	for _, name := range []string{"review.tmpl", "review.md"} {
		path := filepath.Join(pb.cfg.PromptsDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", path, fmt.Errorf("review: prompt: reading template %q: %w", path, err)
		}
		if pb.logger != nil {
			pb.logger.Debug("using custom review template", "path", path)
		}
		return string(data), path, nil
	}

	// Neither custom template exists; use the embedded default.
	return defaultReviewTemplate, "", nil
}

// formatFileList builds the human-readable file list string and returns the
// list of high-risk file paths. If files exceed maxFilesInList, only the first
// maxFilesInList are shown with a trailing note.
func formatFileList(files []ChangedFile) (string, []string) {
	if len(files) == 0 {
		return "", nil
	}

	total := len(files)
	shown := files
	var truncNote string
	if total > maxFilesInList {
		shown = files[:maxFilesInList]
		truncNote = fmt.Sprintf("... and %d more files (showing %d of %d)", total-maxFilesInList, maxFilesInList, total)
	}

	var sb strings.Builder
	var highRisk []string

	for _, f := range shown {
		// Build change summary: e.g. "modified, +42/-10" or "added, +150".
		changeSummary := buildChangeSummary(f)

		if f.Risk == RiskHigh {
			highRisk = append(highRisk, f.Path)
			fmt.Fprintf(&sb, "[HIGH RISK] %s (%s)\n", f.Path, changeSummary)
		} else {
			// Indent non-high-risk files to align with the text after "[HIGH RISK] ".
			fmt.Fprintf(&sb, "            %s (%s)\n", f.Path, changeSummary)
		}
	}

	if truncNote != "" {
		fmt.Fprintf(&sb, "%s\n", truncNote)
	}

	return strings.TrimRight(sb.String(), "\n"), highRisk
}

// buildChangeSummary returns a short string like "modified, +42/-10" or
// "added, +150" describing the file's change type and line deltas.
func buildChangeSummary(f ChangedFile) string {
	changeStr := string(f.ChangeType)
	if f.ChangeType == ChangeRenamed && f.OldPath != "" {
		changeStr = fmt.Sprintf("renamed from %s", f.OldPath)
	}

	switch {
	case f.LinesAdded > 0 && f.LinesDeleted > 0:
		return fmt.Sprintf("%s, +%d/-%d", changeStr, f.LinesAdded, f.LinesDeleted)
	case f.LinesAdded > 0:
		return fmt.Sprintf("%s, +%d", changeStr, f.LinesAdded)
	case f.LinesDeleted > 0:
		return fmt.Sprintf("%s, -%d", changeStr, f.LinesDeleted)
	default:
		return changeStr
	}
}
