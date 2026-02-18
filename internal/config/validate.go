package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// ValidationSeverity indicates whether a validation issue is an error or warning.
type ValidationSeverity string

const (
	// SeverityError indicates a fatal validation issue; the configuration is unusable.
	SeverityError ValidationSeverity = "error"
	// SeverityWarning indicates an informational validation issue; the configuration works
	// but may have problems.
	SeverityWarning ValidationSeverity = "warning"
)

// ValidationIssue represents a single validation finding.
type ValidationIssue struct {
	Severity ValidationSeverity
	Field    string // dotted path, e.g., "project.name"
	Message  string
}

// ValidationResult holds all validation findings.
type ValidationResult struct {
	Issues []ValidationIssue
}

// HasErrors returns true if any issue has error severity.
func (vr *ValidationResult) HasErrors() bool {
	for _, issue := range vr.Issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}

// HasWarnings returns true if any issue has warning severity.
func (vr *ValidationResult) HasWarnings() bool {
	for _, issue := range vr.Issues {
		if issue.Severity == SeverityWarning {
			return true
		}
	}
	return false
}

// Errors returns only error-severity issues.
func (vr *ValidationResult) Errors() []ValidationIssue {
	var errs []ValidationIssue
	for _, issue := range vr.Issues {
		if issue.Severity == SeverityError {
			errs = append(errs, issue)
		}
	}
	return errs
}

// Warnings returns only warning-severity issues.
func (vr *ValidationResult) Warnings() []ValidationIssue {
	var warns []ValidationIssue
	for _, issue := range vr.Issues {
		if issue.Severity == SeverityWarning {
			warns = append(warns, issue)
		}
	}
	return warns
}

// recognizedLanguages is the set of valid values for project.language.
var recognizedLanguages = map[string]bool{
	"":           true,
	"go":         true,
	"typescript": true,
	"python":     true,
	"rust":       true,
	"java":       true,
}

// validEfforts is the set of valid values for agent effort.
var validEfforts = map[string]bool{
	"":       true,
	"low":    true,
	"medium": true,
	"high":   true,
}

// Validate checks the configuration for correctness and completeness.
// It performs structural validation, semantic validation, and unknown key detection.
//
// Parameters:
//   - cfg: the configuration to validate
//   - meta: TOML metadata from BurntSushi/toml (may be nil if no file was loaded)
//
// Returns validation results. Check HasErrors() to determine if the config is usable.
func Validate(cfg *Config, meta *toml.MetaData) *ValidationResult {
	vr := &ValidationResult{}

	if cfg == nil {
		addError(vr, "", "configuration is nil")
		return vr
	}

	validateProject(vr, &cfg.Project)
	validateAgents(vr, cfg.Agents)
	validateReview(vr, &cfg.Review)
	validateWorkflows(vr, cfg.Workflows)
	validateUnknownKeys(vr, meta)

	return vr
}

// validateProject checks the [project] section for errors and warnings.
func validateProject(vr *ValidationResult, p *ProjectConfig) {
	// Error: project.name must not be empty.
	if p.Name == "" {
		addError(vr, "project.name", "must not be empty")
	}

	// Error: project.language must be recognized.
	if !recognizedLanguages[p.Language] {
		addError(vr, "project.language",
			fmt.Sprintf("unrecognized language %q; must be one of: go, typescript, python, rust, java, or empty", p.Language))
	}

	// Error: verification_commands entries must not be empty strings.
	for i, cmd := range p.VerificationCommands {
		if cmd == "" {
			addError(vr, fmt.Sprintf("project.verification_commands[%d]", i),
				"must not be an empty string")
		}
	}

	// Warning: tasks_dir does not exist.
	if p.TasksDir != "" {
		if _, err := os.Stat(p.TasksDir); err != nil {
			addWarning(vr, "project.tasks_dir",
				fmt.Sprintf("directory %q does not exist", p.TasksDir))
		}
	}

	// Warning: log_dir does not exist.
	if p.LogDir != "" {
		if _, err := os.Stat(p.LogDir); err != nil {
			addWarning(vr, "project.log_dir",
				fmt.Sprintf("directory %q does not exist", p.LogDir))
		}
	}
}

// validateAgents checks all [agents.*] sections.
func validateAgents(vr *ValidationResult, agents map[string]AgentConfig) {
	for name, agent := range agents {
		prefix := "agents." + name

		// Error: command must not be empty if agent is defined.
		if agent.Command == "" {
			addError(vr, prefix+".command", "must not be empty")
		}

		// Error: effort must be a recognized value.
		if !validEfforts[agent.Effort] {
			addError(vr, prefix+".effort",
				fmt.Sprintf("unrecognized effort %q; must be one of: low, medium, high, or empty", agent.Effort))
		}

		// Warning: prompt_template file does not exist.
		if agent.PromptTemplate != "" {
			if _, err := os.Stat(agent.PromptTemplate); err != nil {
				addWarning(vr, prefix+".prompt_template",
					fmt.Sprintf("file %q does not exist", agent.PromptTemplate))
			}
		}
	}
}

// validateReview checks the [review] section.
func validateReview(vr *ValidationResult, r *ReviewConfig) {
	// Error: extensions must be a valid regex.
	if r.Extensions != "" {
		if _, err := regexp.Compile(r.Extensions); err != nil {
			addError(vr, "review.extensions",
				fmt.Sprintf("invalid regex %q: %v", r.Extensions, err))
		}
	}

	// Error: risk_patterns must be a valid regex.
	if r.RiskPatterns != "" {
		if _, err := regexp.Compile(r.RiskPatterns); err != nil {
			addError(vr, "review.risk_patterns",
				fmt.Sprintf("invalid regex %q: %v", r.RiskPatterns, err))
		}
	}

	// Warning: prompts_dir does not exist.
	if r.PromptsDir != "" {
		if _, err := os.Stat(r.PromptsDir); err != nil {
			addWarning(vr, "review.prompts_dir",
				fmt.Sprintf("directory %q does not exist", r.PromptsDir))
		}
	}

	// Warning: project_brief_file does not exist.
	if r.ProjectBriefFile != "" {
		if _, err := os.Stat(r.ProjectBriefFile); err != nil {
			addWarning(vr, "review.project_brief_file",
				fmt.Sprintf("file %q does not exist", r.ProjectBriefFile))
		}
	}
}

// validateWorkflows checks all [workflows.*] sections.
func validateWorkflows(vr *ValidationResult, workflows map[string]WorkflowConfig) {
	for name, wf := range workflows {
		prefix := "workflows." + name

		// Error: steps must not be empty.
		if len(wf.Steps) == 0 {
			addError(vr, prefix+".steps", "must not be empty")
			continue
		}

		// Build a set of defined steps for transition validation.
		stepSet := make(map[string]bool, len(wf.Steps))
		for _, step := range wf.Steps {
			stepSet[step] = true
		}

		// Error: transition keys must reference defined steps.
		for stepName, events := range wf.Transitions {
			if !stepSet[stepName] {
				addError(vr, prefix+".transitions."+stepName,
					fmt.Sprintf("references undefined step %q", stepName))
			}
			for event, target := range events {
				if !stepSet[target] {
					addError(vr, prefix+".transitions."+stepName+"."+event,
						fmt.Sprintf("target %q is not a defined step", target))
				}
			}
		}
	}
}

// validateUnknownKeys checks for TOML keys that did not map to any config struct field.
func validateUnknownKeys(vr *ValidationResult, meta *toml.MetaData) {
	if meta == nil {
		return
	}

	for _, key := range meta.Undecoded() {
		path := strings.Join(key, ".")
		addWarning(vr, path, "unknown configuration key")
	}
}

// addError appends an error-severity issue to the validation result.
func addError(vr *ValidationResult, field, message string) {
	vr.Issues = append(vr.Issues, ValidationIssue{
		Severity: SeverityError,
		Field:    field,
		Message:  message,
	})
}

// addWarning appends a warning-severity issue to the validation result.
func addWarning(vr *ValidationResult, field, message string) {
	vr.Issues = append(vr.Issues, ValidationIssue{
		Severity: SeverityWarning,
		Field:    field,
		Message:  message,
	})
}
