# T-033: Review Prompt Synthesis with Project Context Injection

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | T-031, T-032, T-009 |
| Blocked By | T-031, T-032 |
| Blocks | T-035 |

## Goal
Implement the review prompt synthesis engine that constructs agent review prompts from templates, injecting project context (brief, rules, conventions), diff content, file assignments, and structured JSON output instructions. The prompt must guide agents to produce findings in the exact JSON schema defined in T-031.

## Background
Per PRD Section 5.5, review prompts are loaded from template files with project-specific context injection. The `[review]` config section specifies `prompts_dir`, `rules_dir`, and `project_brief_file` which provide project-specific review context. The prompt must instruct agents to output structured JSON findings matching the schema in T-031. In `split` mode, each agent receives only the files assigned to it; in `all` mode, every agent reviews all files.

## Technical Specifications
### Implementation Approach
Create `internal/review/prompt.go` containing a `PromptBuilder` that loads template files from the configured `prompts_dir`, reads project brief and rules files, and renders the final prompt using `text/template`. The template receives the diff content, file list, risk classifications, project context, and JSON schema as template data. Support both `all` and `split` modes by adjusting which files are included in each agent's prompt.

### Key Components
- **PromptBuilder**: Constructs review prompts from templates and project context
- **PromptData**: Template data struct containing all variables available to the template
- **ContextLoader**: Reads project brief, rules files, and conventions from disk
- **Default template**: Embedded fallback review prompt template used when no custom template exists

### API/Interface Contracts
```go
// internal/review/prompt.go

type PromptBuilder struct {
    cfg    ReviewConfig
    loader *ContextLoader
    logger *log.Logger
}

type PromptData struct {
    ProjectBrief   string
    Rules          []string       // Contents of each rule file
    Diff           string         // Full unified diff text
    Files          []ChangedFile  // Files to review (all or split subset)
    FileList       string         // Formatted file list with risk annotations
    HighRiskFiles  []string       // Paths of high-risk files
    Stats          DiffStats      // Diff statistics
    JSONSchema     string         // JSON schema example for findings output
    AgentName      string         // Name of the reviewing agent
    ReviewMode     ReviewMode     // "all" or "split"
}

type ContextLoader struct {
    briefPath string
    rulesDir  string
}

func NewPromptBuilder(cfg ReviewConfig, logger *log.Logger) *PromptBuilder

// Build constructs a review prompt for the given agent and file assignment.
func (pb *PromptBuilder) Build(ctx context.Context, data PromptData) (string, error)

// BuildForAgent constructs a prompt tailored to a specific agent's review assignment.
func (pb *PromptBuilder) BuildForAgent(
    ctx context.Context,
    agentName string,
    diff *DiffResult,
    files []ChangedFile,
    mode ReviewMode,
) (string, error)

// NewContextLoader creates a loader that reads project brief and rules.
func NewContextLoader(briefPath, rulesDir string) *ContextLoader

// Load reads the project brief and all rule files from the rules directory.
func (cl *ContextLoader) Load() (*ProjectContext, error)

type ProjectContext struct {
    Brief string
    Rules []string
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| text/template | stdlib | Template rendering for review prompts |
| os | stdlib | File I/O for loading brief, rules, templates |
| filepath | stdlib | Path manipulation for rules directory walking |
| embed | stdlib | Embedded default review prompt template |
| internal/review (T-031) | - | ReviewConfig, ChangedFile, DiffResult types |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Loads review prompt template from configured `prompts_dir`
- [ ] Falls back to embedded default template when no custom template exists
- [ ] Injects project brief content from `project_brief_file`
- [ ] Loads and injects all rule files from `rules_dir` (*.md files)
- [ ] Includes full unified diff in the prompt
- [ ] Includes formatted file list with risk annotations (high-risk files marked)
- [ ] Includes JSON schema example matching PRD Section 5.5 format
- [ ] In `split` mode, prompt only contains the files assigned to that agent
- [ ] In `all` mode, prompt contains all changed files
- [ ] Prompt instructs agent to output JSON with findings and verdict
- [ ] Unit tests achieve 90% coverage
- [ ] Default embedded template produces valid prompts without any custom configuration

## Testing Requirements
### Unit Tests
- Build prompt with project brief, rules, and diff: all sections present in output
- Build prompt without project brief (file does not exist): prompt still valid, brief section omitted
- Build prompt without rules dir: prompt still valid, rules section omitted
- Build prompt in "all" mode: all files listed
- Build prompt in "split" mode with subset: only assigned files listed
- JSON schema section present and matches expected format
- High-risk files are annotated in the file list
- Template rendering with special characters in diff (Go template escaping)
- Default embedded template produces a complete prompt
- Custom template overrides the default

### Integration Tests
- Full prompt build from a test project directory with brief, rules, and diff
- Prompt is parseable by a human reviewer (structured, readable)

### Edge Cases to Handle
- Empty diff (no changes): prompt should still be valid, explain no changes found
- Very large diff (>100KB): consider truncation with a note
- Rules directory with non-markdown files (should be skipped)
- Project brief file with unicode content
- Template syntax error in custom prompt file: return clear error
- Missing prompts_dir: fall back to embedded default
- Extremely long file list (500+ files): summarize rather than list all

## Implementation Notes
### Recommended Approach
1. Define a default review prompt template as an embedded Go string using `//go:embed`
2. Check if `prompts_dir` contains a `review.md` or `review.tmpl` file; if so, load it
3. Create ContextLoader to read brief and walk rules_dir for *.md files
4. Build PromptData struct with all available context
5. Execute template with PromptData
6. The JSON schema section should include the exact example from PRD Section 5.5
7. For split mode, pass only the agent's assigned files in PromptData.Files

### Potential Pitfalls
- Go `text/template` uses `{{` and `}}` delimiters which conflict with JSON examples in the template. Use raw string blocks or change delimiters with `template.Delims`
- File contents in the diff may contain template-like syntax (`{{`) -- ensure diff is injected as a raw value, not parsed as template
- Rule files should be concatenated with clear separators so the agent can distinguish them
- Large diffs should be truncated with a note rather than silently dropped

### Security Considerations
- Validate that prompts_dir, rules_dir, and project_brief_file paths are within the project directory
- Do not include sensitive files (e.g., `.env`, secrets) even if they appear in the diff
- Sanitize file paths before including them in prompts

## References
- [PRD Section 5.5 - Review prompt synthesis with project context injection](docs/prd/PRD-Raven.md)
- [Go text/template documentation](https://pkg.go.dev/text/template)
- [Go embed documentation](https://pkg.go.dev/embed)
