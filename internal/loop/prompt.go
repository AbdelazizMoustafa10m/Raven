package loop

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/charmbracelet/log"

	"github.com/AbdelazizMoustafa10m/Raven/internal/config"
	"github.com/AbdelazizMoustafa10m/Raven/internal/task"
)

// DefaultImplementTemplate is the built-in prompt template used when no
// custom template file is configured. It uses [[ and ]] as delimiters to
// avoid conflicts with {{ and }} that commonly appear in task spec content
// (e.g. Go template syntax, JSON, shell substitutions).
const DefaultImplementTemplate = `You are implementing task [[.TaskID]]: [[.TaskTitle]]

## Task Specification

[[.TaskSpec]]

## Phase Context

Phase [[.PhaseID]]: [[.PhaseName]] ([[.PhaseRange]])

## Project

Project: [[.ProjectName]] ([[.ProjectLanguage]])

## Verification Commands

After making changes, run these commands to verify:
[[range .VerificationCommands]]
- [[.]]
[[end]]

## Progress

Completed tasks: [[.CompletedSummary]]
Remaining tasks: [[.RemainingSummary]]

## Instructions

1. Read the task specification carefully
2. Implement the changes described in the acceptance criteria
3. Run the verification commands to ensure correctness
4. When the task is complete, output PHASE_COMPLETE
5. If the task is blocked, output TASK_BLOCKED with a reason
6. If you encounter an unrecoverable error, output RAVEN_ERROR with details
`

// PromptContext holds all runtime values that are substituted into a prompt
// template when generating an agent prompt.
type PromptContext struct {
	// Task-specific context.
	TaskSpec  string // Full markdown content of the current task spec.
	TaskID    string // e.g., "T-016"
	TaskTitle string // e.g., "Task Spec Markdown Parser"

	// Phase context.
	PhaseID    int    // Current phase number.
	PhaseName  string // e.g., "Task System & Agent Adapters"
	PhaseRange string // e.g., "T-016 to T-030"

	// Project context.
	ProjectName     string // From raven.toml project.name.
	ProjectLanguage string // From raven.toml project.language.

	// Verification.
	VerificationCommands []string // From raven.toml project.verification_commands.
	VerificationString   string   // Commands joined with " && ".

	// Task progress context.
	CompletedTasks   []string // IDs of completed tasks.
	RemainingTasks   []string // IDs of remaining tasks in current phase.
	CompletedSummary string   // Formatted summary of completed tasks.
	RemainingSummary string   // Formatted summary of remaining tasks.

	// Agent context.
	AgentName string // e.g., "claude"
	Model     string // e.g., "claude-opus-4-6"
}

// PromptGenerator loads, caches, and renders prompt templates. It uses
// [[ and ]] as template delimiters so that {{ and }} in task spec content
// are never misinterpreted as template actions.
type PromptGenerator struct {
	templateDir string
	templates   map[string]*template.Template
	defaultTmpl *template.Template
}

// NewPromptGenerator creates a PromptGenerator. If templateDir is non-empty,
// it must refer to an existing directory; an error is returned otherwise.
// The built-in DefaultImplementTemplate is pre-parsed and cached as the
// fallback template.
func NewPromptGenerator(templateDir string) (*PromptGenerator, error) {
	if templateDir != "" {
		info, err := os.Stat(templateDir)
		if err != nil {
			return nil, fmt.Errorf("prompt generator: template directory %q: %w", templateDir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("prompt generator: template directory %q is not a directory", templateDir)
		}
	}

	defaultTmpl, err := template.New("default").
		Delims("[[", "]]").
		Parse(DefaultImplementTemplate)
	if err != nil {
		// The default template is a compile-time constant; any parse error is a
		// programming bug, so we surface a clear message.
		return nil, fmt.Errorf("prompt generator: parsing default template: %w", err)
	}

	return &PromptGenerator{
		templateDir: templateDir,
		templates:   make(map[string]*template.Template),
		defaultTmpl: defaultTmpl,
	}, nil
}

// LoadTemplate loads the named template file from the generator's templateDir,
// parses it with [[ / ]] delimiters, and caches the result. Subsequent calls
// for the same name return the cached template without re-reading the file.
//
// The name must not be empty and must not contain path components that would
// escape the templateDir (directory traversal is rejected).
func (pg *PromptGenerator) LoadTemplate(name string) (*template.Template, error) {
	if name == "" {
		return nil, fmt.Errorf("loading template: name must not be empty")
	}
	if pg.templateDir == "" {
		return nil, fmt.Errorf("loading template %q: no template directory configured", name)
	}

	// Return cached copy if available.
	if tmpl, ok := pg.templates[name]; ok {
		return tmpl, nil
	}

	// Security: resolve the full path and verify it stays within templateDir.
	absDir, err := filepath.Abs(pg.templateDir)
	if err != nil {
		return nil, fmt.Errorf("loading template %q: resolving template directory: %w", name, err)
	}
	candidate := filepath.Join(absDir, name)
	// filepath.Rel returns an error or a path starting with ".." when candidate
	// is outside absDir.
	rel, err := filepath.Rel(absDir, candidate)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("loading template %q: path escapes template directory (directory traversal rejected)", name)
	}

	raw, err := os.ReadFile(candidate)
	if err != nil {
		return nil, fmt.Errorf("loading template %q: %w", name, err)
	}

	tmpl, err := template.New(name).
		Delims("[[", "]]").
		Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("loading template %q: parsing: %w", name, err)
	}

	pg.templates[name] = tmpl
	log.Debug("loaded prompt template", "name", name, "path", candidate)
	return tmpl, nil
}

// Generate renders a prompt for the given PromptContext. If templateName is
// non-empty, the named template file is loaded (and cached) from the
// generator's templateDir. If templateName is empty, the built-in
// DefaultImplementTemplate is used.
func (pg *PromptGenerator) Generate(templateName string, ctx PromptContext) (string, error) {
	var tmpl *template.Template
	var err error

	if templateName == "" {
		tmpl = pg.defaultTmpl
	} else {
		tmpl, err = pg.LoadTemplate(templateName)
		if err != nil {
			return "", fmt.Errorf("generating prompt with template %q: %w", templateName, err)
		}
	}

	return pg.execute(tmpl, ctx)
}

// GenerateFromString renders a prompt from an inline template string rather
// than a file. The string must use [[ and ]] as delimiters.
func (pg *PromptGenerator) GenerateFromString(tmplStr string, ctx PromptContext) (string, error) {
	tmpl, err := template.New("inline").
		Delims("[[", "]]").
		Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("generating prompt from inline string: parsing template: %w", err)
	}
	return pg.execute(tmpl, ctx)
}

// execute renders tmpl with ctx and returns the resulting string.
// The rendered prompt is logged at debug level.
func (pg *PromptGenerator) execute(tmpl *template.Template, ctx PromptContext) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("executing prompt template %q: %w", tmpl.Name(), err)
	}
	result := buf.String()
	log.Debug("rendered prompt template",
		"template", tmpl.Name(),
		"task", ctx.TaskID,
		"agent", ctx.AgentName,
		"bytes", len(result),
	)
	return result, nil
}

// BuildContext constructs a PromptContext from the provided parsed task spec,
// phase, project configuration, task selector, and agent name.
//
// The selector is used to populate CompletedTasks and RemainingTasks. The
// agentName is matched against cfg.Agents to look up the configured model.
func BuildContext(
	spec *task.ParsedTaskSpec,
	phase *task.Phase,
	cfg *config.Config,
	selector *task.TaskSelector,
	agentName string,
) (*PromptContext, error) {
	if spec == nil {
		return nil, fmt.Errorf("building prompt context: spec must not be nil")
	}
	if phase == nil {
		return nil, fmt.Errorf("building prompt context: phase must not be nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("building prompt context: cfg must not be nil")
	}
	if selector == nil {
		return nil, fmt.Errorf("building prompt context: selector must not be nil")
	}

	completedIDs, err := selector.CompletedTaskIDs()
	if err != nil {
		return nil, fmt.Errorf("building prompt context: listing completed tasks: %w", err)
	}

	remainingIDs, err := selector.RemainingTaskIDs(phase.ID)
	if err != nil {
		return nil, fmt.Errorf("building prompt context: listing remaining tasks: %w", err)
	}

	// Build VerificationString from the slice (no trailing " && ").
	verifyStr := strings.Join(cfg.Project.VerificationCommands, " && ")

	// Determine model from agent configuration.
	model := ""
	if agentCfg, ok := cfg.Agents[agentName]; ok {
		model = agentCfg.Model
	}

	ctx := &PromptContext{
		// Task fields.
		TaskSpec:  spec.Content,
		TaskID:    spec.ID,
		TaskTitle: spec.Title,

		// Phase fields.
		PhaseID:    phase.ID,
		PhaseName:  phase.Name,
		PhaseRange: phase.StartTask + " to " + phase.EndTask,

		// Project fields.
		ProjectName:          cfg.Project.Name,
		ProjectLanguage:      cfg.Project.Language,
		VerificationCommands: cfg.Project.VerificationCommands,
		VerificationString:   verifyStr,

		// Progress fields.
		CompletedTasks:   completedIDs,
		RemainingTasks:   remainingIDs,
		CompletedSummary: formatIDSummary(completedIDs),
		RemainingSummary: formatIDSummary(remainingIDs),

		// Agent fields.
		AgentName: agentName,
		Model:     model,
	}

	return ctx, nil
}

// formatIDSummary returns "None" when ids is empty, otherwise the IDs joined
// with ", ".
func formatIDSummary(ids []string) string {
	if len(ids) == 0 {
		return "None"
	}
	return strings.Join(ids, ", ")
}
