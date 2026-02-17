# T-026: Prompt Template System

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-004, T-009, T-016, T-017, T-018, T-019 |
| Blocked By | T-016, T-019 |
| Blocks | T-027, T-029 |

## Goal
Implement the prompt template system that generates agent prompts by substituting placeholders with runtime values (task spec content, phase info, completed tasks, verification commands, etc.). This system bridges the gap between static prompt template files and the dynamic context needed for each agent invocation, enabling consistent and context-rich prompts across all workflow stages.

## Background
Per PRD Section 5.4, the implementation loop uses prompt templates with these placeholders:
- `{{TASK_SPEC}}` -- contents of the task markdown file
- `{{PHASE_INFO}}` -- current phase name and range
- `{{PROJECT_NAME}}` -- from `raven.toml`
- `{{VERIFICATION_COMMANDS}}` -- from `raven.toml`
- `{{COMPLETED_TASKS}}` -- list of already-completed tasks for context
- `{{REMAINING_TASKS}}` -- list of remaining tasks

Prompt templates are loaded from files in the configurable `prompt_dir` directory. Each agent can have its own prompt template (`prompt_template` field in agent config). The system uses Go's `text/template` for robust template rendering with support for conditionals and iteration.

## Technical Specifications
### Implementation Approach
Create `internal/loop/prompt.go` containing a `PromptGenerator` that loads template files, populates a data context from the task system and configuration, and renders the final prompt string. Use Go's `text/template` package for template rendering. The generator should support both file-based templates and inline template strings. Provide a `PromptContext` struct that holds all the data available for template substitution.

### Key Components
- **PromptGenerator**: Loads templates and renders prompts with runtime context
- **PromptContext**: Data structure holding all template variables
- **Template loading**: Reads template files from disk with fallback to embedded defaults
- **Default template**: A built-in implementation prompt template used when no custom template exists

### API/Interface Contracts
```go
// internal/loop/prompt.go
package loop

import (
    "text/template"
)

// PromptContext holds all data available for prompt template substitution.
type PromptContext struct {
    // Task-specific context
    TaskSpec       string   // Full markdown content of the current task spec
    TaskID         string   // e.g., "T-016"
    TaskTitle      string   // e.g., "Task Spec Markdown Parser"

    // Phase context
    PhaseID        int      // Current phase number
    PhaseName      string   // e.g., "Task System & Agent Adapters"
    PhaseRange     string   // e.g., "T-016 to T-030"

    // Project context
    ProjectName    string   // From raven.toml project.name
    ProjectLanguage string  // From raven.toml project.language

    // Verification
    VerificationCommands []string // From raven.toml project.verification_commands
    VerificationString   string   // Commands joined with " && "

    // Task progress context
    CompletedTasks  []string // IDs of completed tasks
    RemainingTasks  []string // IDs of remaining tasks in current phase
    CompletedSummary string  // Formatted summary of completed tasks
    RemainingSummary string  // Formatted summary of remaining tasks

    // Agent context
    AgentName      string   // e.g., "claude"
    Model          string   // e.g., "claude-opus-4-6"
}

// PromptGenerator generates prompts from templates and runtime context.
type PromptGenerator struct {
    templateDir string               // Directory containing template files
    templates   map[string]*template.Template // Cached parsed templates
    defaultTmpl *template.Template   // Fallback template
}

// NewPromptGenerator creates a generator that loads templates from the given directory.
func NewPromptGenerator(templateDir string) (*PromptGenerator, error)

// LoadTemplate loads and parses a template file by name.
// Returns cached version if already loaded.
func (pg *PromptGenerator) LoadTemplate(name string) (*template.Template, error)

// Generate renders a prompt using the specified template and context.
// If templateName is empty, uses the default built-in template.
func (pg *PromptGenerator) Generate(templateName string, ctx PromptContext) (string, error)

// GenerateFromString renders a prompt from an inline template string.
func (pg *PromptGenerator) GenerateFromString(tmplStr string, ctx PromptContext) (string, error)

// BuildContext creates a PromptContext from the task system components.
// This is a convenience method that populates all fields from the
// task selector, config, and current state.
func BuildContext(
    spec *task.ParsedTaskSpec,
    phase *task.Phase,
    cfg *config.Config,
    selector *task.TaskSelector,
    agentName string,
) (*PromptContext, error)

// DefaultImplementTemplate is the built-in prompt template used when
// no custom template file exists.
const DefaultImplementTemplate = `You are implementing task {{.TaskID}}: {{.TaskTitle}}

## Task Specification

{{.TaskSpec}}

## Phase Context

Phase {{.PhaseID}}: {{.PhaseName}} ({{.PhaseRange}})

## Project

Project: {{.ProjectName}} ({{.ProjectLanguage}})

## Verification Commands

After making changes, run these commands to verify:
{{range .VerificationCommands}}
- {{.}}
{{end}}

## Progress

Completed tasks: {{.CompletedSummary}}
Remaining tasks: {{.RemainingSummary}}

## Instructions

1. Read the task specification carefully
2. Implement the changes described in the acceptance criteria
3. Run the verification commands to ensure correctness
4. When the task is complete, output PHASE_COMPLETE
5. If the task is blocked, output TASK_BLOCKED with a reason
6. If you encounter an unrecoverable error, output RAVEN_ERROR with details
`
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| text/template | stdlib | Template parsing and rendering |
| os | stdlib | Template file reading |
| strings | stdlib | String joining for verification commands |
| path/filepath | stdlib | Template file path resolution |
| internal/task (T-016) | - | ParsedTaskSpec for task context |
| internal/task (T-018) | - | Phase for phase context |
| internal/task (T-019) | - | TaskSelector for progress context |
| internal/config (T-009) | - | Config for project context |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] PromptGenerator loads template files from configurable directory
- [ ] Templates are cached after first load (not re-read from disk)
- [ ] Generate renders all placeholders correctly: TaskSpec, PhaseInfo, ProjectName, VerificationCommands, CompletedTasks, RemainingTasks
- [ ] Default template is used when no custom template file exists
- [ ] GenerateFromString works with inline template strings
- [ ] BuildContext populates all PromptContext fields from task system components
- [ ] Verification commands are joined with " && " for VerificationString
- [ ] CompletedSummary and RemainingSummary are human-readable formatted strings
- [ ] Template errors (bad syntax, missing fields) return clear error messages
- [ ] Unit tests achieve 90% coverage
- [ ] All exported types and functions have doc comments

## Testing Requirements
### Unit Tests
- Generate with default template and complete context: all placeholders substituted
- Generate with custom template file: custom template used
- Generate with missing template file: falls back to default
- GenerateFromString with simple template: correct output
- Template with {{.TaskSpec}} containing markdown: rendered without escaping
- Template with {{.VerificationCommands}} range: each command on its own line
- Template with empty CompletedTasks: "None" or empty section
- Template with empty RemainingTasks: "None" or empty section
- BuildContext from task components: all fields populated correctly
- BuildContext with no phase (--task mode): phase fields empty/default
- Template syntax error: returns descriptive error
- Template referencing undefined field: returns error at render time
- LoadTemplate caching: second call returns same template object

### Integration Tests
- Generate prompt using actual task spec from testdata and real config

### Edge Cases to Handle
- Task spec containing Go template syntax (`{{`, `}}`): must not be interpreted as template directives. Use `{{` literal escaping or raw string handling
- Very large task spec (>50KB): template rendering should handle without issues
- Template file with Windows line endings: normalize
- Empty template file: return error
- Template directory does not exist: return clear error
- PromptContext with nil slices: template should handle gracefully (empty range)

## Implementation Notes
### Recommended Approach
1. Define PromptContext struct with all fields first
2. Define DefaultImplementTemplate as a const string
3. NewPromptGenerator: validate templateDir exists (or create), parse default template
4. LoadTemplate: `filepath.Join(templateDir, name)`, parse with `template.ParseFiles`, cache in map
5. Generate: load template, execute with PromptContext, capture output to `bytes.Buffer`
6. BuildContext: call selector.CompletedTaskIDs(), selector.RemainingTaskIDs(), format summaries
7. For the task spec escaping issue: preprocess TaskSpec to escape `{{` before template rendering, or use a custom delimiter (not recommended -- adds complexity)
8. Alternative for task spec: use `{{.TaskSpec | raw}}` with a custom `raw` function, or simply use string replacement (strings.ReplaceAll) for the known placeholders instead of text/template
9. Pragmatic approach: use `strings.NewReplacer` for the simple `{{PLACEHOLDER}}` format from the PRD, and reserve `text/template` for more complex templates

### Potential Pitfalls
- **Critical**: Task spec content may contain `{{` and `}}` which Go's text/template will try to interpret. Solutions:
  1. Use `strings.NewReplacer` for simple substitution (safest, matches PRD's `{{PLACEHOLDER}}` format)
  2. Or escape `{{` as `{{"{{"}}` in the task spec before rendering
  3. Recommendation: Use `strings.NewReplacer` for the PRD's placeholder format, and offer `text/template` rendering as an advanced option for custom templates
- Template rendering is CPU-bound for large contexts -- this is fine for CLI usage
- Do not use `html/template` (it HTML-escapes output, which is wrong for CLI prompts)
- Ensure VerificationString does not have trailing " && " for the last command

### Security Considerations
- Prompts may contain proprietary code from task specs -- only log rendered prompts at debug level
- Template rendering should not execute arbitrary code -- text/template is safe in this regard (no FuncMap with dangerous functions)
- Validate template file paths to prevent directory traversal

## References
- [PRD Section 5.4 - Prompt template placeholders](docs/prd/PRD-Raven.md)
- [Go text/template documentation](https://pkg.go.dev/text/template)
- [Go text/template tutorial](https://gobyexample.com/text-templates)
- [valyala/fasttemplate - Simple template engine](https://github.com/valyala/fasttemplate)
