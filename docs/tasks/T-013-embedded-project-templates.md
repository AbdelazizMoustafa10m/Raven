# T-013: Embedded Project Templates -- go-cli Template

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 4-8hrs |
| Dependencies | T-001 |
| Blocked By | T-001 |
| Blocks | T-014, T-083, T-084 |

## Goal
Create the `go-cli` project template and the template embedding infrastructure using Go's `//go:embed` directive. The template includes a complete `raven.toml`, prompt files, review configuration directory structure, and task directory scaffolding. This is the foundation for `raven init go-cli` (T-014) and future templates.

## Background
Per PRD Section 5.10, `raven init [template]` scaffolds a new project. The `go-cli` template is the first and primary template, providing a complete Raven configuration for Go CLI projects. Per PRD Section 6.2, templates live in the `templates/` directory and are embedded in the binary using `//go:embed`. Per PRD Section 5.10, future templates (node-service, python-lib) are planned for v2.1.

The template system must support both static files (copied as-is) and template files (processed with `text/template` for variable substitution like project name). The distinction is important: `raven.toml` contains `{{.ProjectName}}` placeholders, while `.gitkeep` files are static.

## Technical Specifications
### Implementation Approach
Create the `templates/go-cli/` directory tree with all template files. Create `internal/config/templates.go` with the `//go:embed` directive and functions to list available templates, read template files, and process template variables. Use Go's `embed.FS` for the embedded filesystem and `text/template` for variable substitution.

### Key Components
- **templates/go-cli/**: Complete Go CLI project template directory
- **TemplateFS**: Embedded filesystem containing all templates
- **ListTemplates()**: Returns available template names
- **GetTemplate()**: Returns the embedded FS for a specific template
- **TemplateVars**: Variables available for template substitution
- **ProcessFile()**: Applies text/template processing to a file

### API/Interface Contracts
```go
// internal/config/templates.go

package config

import "embed"

//go:embed all:templates
var templateFS embed.FS

// TemplateVars holds variables available for template substitution.
type TemplateVars struct {
    ProjectName string
    Language    string
    ModulePath  string // Go module path
}

// ListTemplates returns the names of all available project templates.
func ListTemplates() ([]string, error)

// TemplateExists checks if a template with the given name exists.
func TemplateExists(name string) bool

// RenderTemplate writes the template files to the destination directory,
// processing any .tmpl files with text/template substitution.
// Files without .tmpl extension are copied as-is.
// Returns the list of files created.
func RenderTemplate(name string, destDir string, vars TemplateVars) ([]string, error)
```

Note: The `//go:embed` directive must be in a Go file at or above the `templates/` directory. Since `templates/` is at the project root and the Go package is in `internal/config/`, we need to either:
1. Place the embed directive in `cmd/raven/` and pass the FS down, or
2. Move the templates into `internal/config/templates/`

Option 2 is cleaner -- embed the templates directory within the config package.

### Template Directory Structure
```
internal/config/templates/go-cli/
    raven.toml.tmpl
    prompts/
        implement-claude.md
        implement-codex.md
    .github/
        review/
            prompts/
                review-prompt.md
            rules/
                .gitkeep
            PROJECT_BRIEF.md.tmpl
    docs/
        tasks/
            .gitkeep
        prd/
            .gitkeep
```

### Template File Content

**raven.toml.tmpl:**
```toml
[project]
name = "{{.ProjectName}}"
language = "{{.Language}}"
tasks_dir = "docs/tasks"
task_state_file = "docs/tasks/task-state.conf"
phases_conf = "docs/tasks/phases.conf"
progress_file = "docs/tasks/PROGRESS.md"
log_dir = "scripts/logs"
prompt_dir = "prompts"
branch_template = "phase/{phase_id}-{slug}"
verification_commands = [
    "go build ./...",
    "go vet ./...",
    "go test ./...",
    "go mod tidy"
]

[agents.claude]
command = "claude"
model = "claude-opus-4-6"
effort = "high"
prompt_template = "prompts/implement-claude.md"
allowed_tools = "Edit,Write,Read,Glob,Grep,Bash(go*),Bash(git*)"

[agents.codex]
command = "codex"
model = "gpt-5.3-codex"
effort = "high"
prompt_template = "prompts/implement-codex.md"

[review]
extensions = '(\.go$|go\.mod$|go\.sum$)'
risk_patterns = '^(cmd/|internal/|scripts/)'
prompts_dir = ".github/review/prompts"
rules_dir = ".github/review/rules"
project_brief_file = ".github/review/PROJECT_BRIEF.md"
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| embed | stdlib (Go 1.16+) | File embedding in binary |
| text/template | stdlib | Template variable substitution |
| io/fs | stdlib | Embedded filesystem traversal |
| os | stdlib | File creation and directory creation |

## Acceptance Criteria
- [ ] `internal/config/templates/go-cli/` directory exists with all template files
- [ ] `//go:embed` directive successfully embeds the templates directory
- [ ] `ListTemplates()` returns `["go-cli"]`
- [ ] `TemplateExists("go-cli")` returns true
- [ ] `TemplateExists("nonexistent")` returns false
- [ ] `RenderTemplate("go-cli", destDir, vars)` creates all template files in destDir
- [ ] `.tmpl` files are processed with text/template substitution
- [ ] Non-`.tmpl` files are copied as-is (binary-safe)
- [ ] `raven.toml.tmpl` is rendered as `raven.toml` (extension stripped)
- [ ] Template variables (ProjectName, Language) are substituted correctly
- [ ] Directory structure is preserved (prompts/, .github/review/, docs/tasks/)
- [ ] Empty directories are created (via .gitkeep files)
- [ ] Template rendering does not overwrite existing files (returns error or skips)
- [ ] Unit tests achieve 90% coverage
- [ ] `go build ./...` compiles (embed works)
- [ ] `go vet ./...` passes

## Testing Requirements
### Unit Tests
- ListTemplates returns ["go-cli"]
- TemplateExists("go-cli") returns true
- TemplateExists("unknown") returns false
- RenderTemplate creates raven.toml with substituted project name
- RenderTemplate creates prompts/ directory
- RenderTemplate creates .github/review/ directory structure
- RenderTemplate with custom ProjectName: raven.toml contains that name
- RenderTemplate to non-existent destDir: creates the directory
- RenderTemplate with invalid template name: returns error
- Template file count matches expected (verify all files are embedded)

### Integration Tests
- RenderTemplate to t.TempDir() and verify all files exist with correct content
- Rendered raven.toml is valid TOML (parse with BurntSushi/toml)

### Edge Cases to Handle
- Destination directory already exists with some files: RenderTemplate should not overwrite
- Template file with no variables (static): should be copied unchanged
- Template file with syntax error: text/template returns parse error (caught at render time)
- Nested .gitkeep files: should be created to preserve directory structure
- File permissions: created files should have 0644, directories 0755
- Template variables with special characters (e.g., project name with spaces): should be escaped in TOML

## Implementation Notes
### Recommended Approach
1. Create `internal/config/templates/go-cli/` directory structure with all files
2. Create `internal/config/templates.go` with `//go:embed all:templates` directive
3. Implement `ListTemplates()` by reading the top-level directories from the embed.FS
4. Implement `TemplateExists()` by checking if the directory exists in embed.FS
5. Implement `RenderTemplate()`:
   a. Walk the template directory in embed.FS using `fs.WalkDir`
   b. For each file: if it ends with `.tmpl`, process with `text/template` and strip extension
   c. For non-`.tmpl` files: copy bytes as-is
   d. Create parent directories as needed
   e. Return list of created file paths
6. Create template content files (raven.toml.tmpl, prompt files, etc.)
7. Verify: `go build ./... && go test ./internal/config/...`

### Potential Pitfalls
- The `//go:embed` directive requires the `all:` prefix to include files starting with `.` (like `.github/` and `.gitkeep`). Without `all:`, dotfiles are excluded.
- The embed path is relative to the Go source file containing the directive. Since the directive is in `internal/config/templates.go`, the path is `templates` (referring to `internal/config/templates/`).
- `text/template` uses `{{.Field}}` syntax which conflicts with shell variables and TOML inline tables. Escape or avoid conflicts in template content.
- `embed.FS` paths always use forward slashes, even on Windows.
- Do not embed large binary files -- templates should be small text files (PRD Section 9: "Templates are small text files").

### Security Considerations
- Embedded templates are read-only and cannot be tampered with at runtime
- Template rendering should not follow symlinks in the destination directory
- The TemplateVars.ProjectName should be validated before use in templates to prevent injection

## References
- [Go embed Package Documentation](https://pkg.go.dev/embed)
- [Go embed Directive Specification](https://pkg.go.dev/embed#hdr-Directives)
- [text/template Package](https://pkg.go.dev/text/template)
- [PRD Section 5.10 - raven init templates](docs/prd/PRD-Raven.md)
- [PRD Section 6.2 - templates/ directory](docs/prd/PRD-Raven.md)
- [Using Go Embed for Templates](https://andrew-mccall.com/blog/2025/01/using-go-embed-package-for-template-rendering/)