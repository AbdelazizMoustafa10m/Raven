# T-014: Init Command -- raven init [template]

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 4-6hrs |
| Dependencies | T-006, T-013 |
| Blocked By | T-006, T-013 |
| Blocks | None |

## Goal
Implement the `raven init [template]` command that scaffolds a new Raven project using an embedded template. The command creates the directory structure, configuration file, prompt templates, and review setup -- getting a developer from zero to a working Raven configuration in under 5 minutes (PRD Section 4 success metric).

## Background
Per PRD Section 5.10, `raven init [template]` scaffolds a new project. The `go-cli` template is the primary template for v2.0. Per PRD Section 7 (Phase 1), `raven init go-cli` is one of the three Phase 1 deliverables. The command is special -- it does not require an existing `raven.toml` (unlike all other commands that load config). It works in the current directory or a directory specified by `--dir`.

Per the bash prototype (`bin/raven`, line 15-16): "init is special -- no raven.toml needed." The Go implementation follows the same pattern -- the `init` command skips config loading in its PreRun hook.

## Technical Specifications
### Implementation Approach
Create `internal/cli/init_cmd.go` with a Cobra command that accepts an optional template name argument (defaults to listing available templates if not provided). The command validates the template exists, prompts for a project name if not provided via `--name`, checks for existing `raven.toml` (to prevent accidental overwrite), renders the template using the infrastructure from T-013, and reports created files.

### Key Components
- **initCmd**: Cobra command for `raven init [template]`
- **--name flag**: Project name (defaults to directory name)
- **--force flag**: Overwrite existing files
- **Template rendering**: Delegates to `config.RenderTemplate()` from T-013
- **File conflict detection**: Checks if raven.toml already exists
- **Success output**: Lists all created files

### API/Interface Contracts
```go
// internal/cli/init_cmd.go

package cli

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/spf13/cobra"
    "github.com/charmbracelet/log"

    "<module>/internal/config"
)

var (
    initName  string
    initForce bool
)

var initCmd = &cobra.Command{
    Use:   "init [template]",
    Short: "Initialize a new Raven project",
    Long: `Initialize a new Raven project with configuration, prompts, and directory structure.

Available templates:
  go-cli       Go CLI project (default)

Example:
  raven init go-cli
  raven init go-cli --name my-project
  raven init --name my-api  (defaults to go-cli template)`,
    Args: cobra.MaximumNArgs(1),
    // Override PersistentPreRunE to skip config loading
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        // Only initialize logging, do NOT load config
        jsonFormat := os.Getenv("RAVEN_LOG_FORMAT") == "json"
        logging.Setup(flagVerbose, flagQuiet, jsonFormat)
        return nil
    },
    RunE: func(cmd *cobra.Command, args []string) error {
        // Determine template name
        templateName := "go-cli"
        if len(args) > 0 {
            templateName = args[0]
        }

        // Validate template exists
        if !config.TemplateExists(templateName) {
            templates, _ := config.ListTemplates()
            return fmt.Errorf("unknown template %q, available: %v", templateName, templates)
        }

        // Determine destination directory
        destDir, err := os.Getwd()
        if err != nil {
            return fmt.Errorf("getting working directory: %w", err)
        }

        // Determine project name
        if initName == "" {
            initName = filepath.Base(destDir)
        }

        // Check for existing raven.toml
        if !initForce {
            if _, err := os.Stat(filepath.Join(destDir, "raven.toml")); err == nil {
                return fmt.Errorf("raven.toml already exists (use --force to overwrite)")
            }
        }

        // Render template
        vars := config.TemplateVars{
            ProjectName: initName,
            Language:    languageForTemplate(templateName),
        }

        files, err := config.RenderTemplate(templateName, destDir, vars)
        if err != nil {
            return fmt.Errorf("rendering template: %w", err)
        }

        // Report success
        fmt.Fprintf(os.Stderr, "Initialized Raven project %q with template %q\n\n", initName, templateName)
        fmt.Fprintf(os.Stderr, "Created files:\n")
        for _, f := range files {
            rel, _ := filepath.Rel(destDir, f)
            fmt.Fprintf(os.Stderr, "  %s\n", rel)
        }
        fmt.Fprintf(os.Stderr, "\nNext steps:\n")
        fmt.Fprintf(os.Stderr, "  1. Edit raven.toml to customize your configuration\n")
        fmt.Fprintf(os.Stderr, "  2. Run 'raven config validate' to verify\n")
        fmt.Fprintf(os.Stderr, "  3. Run 'raven config debug' to see resolved config\n")

        return nil
    },
}

func languageForTemplate(template string) string {
    switch template {
    case "go-cli":
        return "go"
    default:
        return ""
    }
}

func init() {
    initCmd.Flags().StringVar(&initName, "name", "", "Project name (defaults to directory name)")
    initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing files")
    rootCmd.AddCommand(initCmd)
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| github.com/spf13/cobra | v1.10+ | CLI framework |
| internal/config | - | Template rendering (T-013) |
| internal/logging | - | Logging setup (T-005) |

## Acceptance Criteria
- [ ] `raven init go-cli` creates all template files in the current directory
- [ ] `raven init go-cli --name my-project` uses "my-project" as the project name
- [ ] `raven init` with no template argument defaults to "go-cli"
- [ ] `raven init` with no `--name` flag uses the current directory name
- [ ] `raven init go-cli` in a directory with existing `raven.toml` returns error
- [ ] `raven init go-cli --force` overwrites existing files
- [ ] `raven init unknown-template` returns error listing available templates
- [ ] Created `raven.toml` contains the project name
- [ ] Created `raven.toml` is valid TOML
- [ ] Created directory structure includes prompts/, .github/review/, docs/tasks/
- [ ] Success output lists all created files
- [ ] Success output shows next steps
- [ ] Command does NOT require existing raven.toml (skips config loading)
- [ ] Command respects `--dir` global flag for destination directory
- [ ] Exit code is 0 on success, 1 on error
- [ ] `go vet ./...` passes
- [ ] Unit tests achieve 85% coverage

## Testing Requirements
### Unit Tests
- initCmd with "go-cli" template creates raven.toml
- initCmd with --name flag sets project name in raven.toml
- initCmd without --name uses directory name
- initCmd with existing raven.toml returns error
- initCmd with --force and existing raven.toml succeeds
- initCmd with invalid template returns error with available templates listed
- initCmd with no arguments defaults to go-cli
- Created raven.toml parses as valid TOML
- languageForTemplate("go-cli") returns "go"

### Integration Tests
- Full end-to-end: `raven init go-cli --name test-project` in t.TempDir(), verify all files
- Rendered raven.toml loads successfully with config.LoadFromFile

### Edge Cases to Handle
- Current directory is read-only: RenderTemplate returns permission error
- Project name with special characters (spaces, slashes): should be sanitized or rejected
- Empty directory name (root "/"): filepath.Base returns "/"
- Template rendering partially fails (first file created, second fails): no automatic cleanup (document this)
- Running init in a git repository: should not affect git state

## Implementation Notes
### Recommended Approach
1. Create `internal/cli/init_cmd.go`
2. Define `initCmd` with `MaximumNArgs(1)` for optional template name
3. Override `PersistentPreRunE` to skip config loading (only setup logging)
4. Implement RunE with template validation, conflict checking, and rendering
5. Include "next steps" guidance in success output
6. Register via `rootCmd.AddCommand(initCmd)` in `init()`
7. Create `internal/cli/init_cmd_test.go` using `t.TempDir()` for isolation
8. Test end-to-end: `make build && cd /tmp && mkdir test && cd test && ../path/to/raven init go-cli`

### Potential Pitfalls
- The `init` command MUST override `PersistentPreRunE` to skip config loading. Without this, the root command's PersistentPreRunE might try to find raven.toml (which does not exist yet) and error. Since init overrides PersistentPreRunE, it must still call `logging.Setup()` manually.
- `cobra.Command` name is `init` which is a Go keyword. This is fine for cobra -- the command struct variable should be named `initCmd` (matching the convention in CLAUDE.md and PRD).
- The `--dir` global flag changes CWD in the root PersistentPreRunE. Since init overrides PersistentPreRunE, it must handle `--dir` itself if needed, or the user should cd to the target directory.
- Status output goes to stderr (per PRD convention), not stdout.

### Security Considerations
- Template rendering writes files to the filesystem. Ensure path traversal is not possible via template variable injection (e.g., project name containing "../").
- The `--force` flag should be used deliberately -- warn the user about overwriting.
- Created files should have standard permissions (0644 for files, 0755 for directories).

## References
- [PRD Section 5.10 - raven init templates](docs/prd/PRD-Raven.md)
- [PRD Section 7 - Phase 1 Deliverable: raven init go-cli](docs/prd/PRD-Raven.md)
- [PRD Section 4 - Success Metric: < 5 minutes to configure](docs/prd/PRD-Raven.md)
- [Go embed.FS Documentation](https://pkg.go.dev/embed#FS)
- [Cobra Command Documentation](https://pkg.go.dev/github.com/spf13/cobra#Command)