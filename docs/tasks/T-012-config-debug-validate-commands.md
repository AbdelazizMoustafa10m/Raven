# T-012: Config Debug and Validate Commands -- raven config debug/validate

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 4-6hrs |
| Dependencies | T-006, T-009, T-010, T-011 |
| Blocked By | T-006, T-009, T-010, T-011 |
| Blocks | None |

## Goal
Implement the `raven config debug` and `raven config validate` subcommands. `config debug` displays the fully-resolved configuration with source annotations (showing which value came from CLI, env, file, or default). `config validate` runs the validation suite and reports errors and warnings. These are essential debugging tools and a Phase 1 deliverable.

## Background
Per PRD Section 5.10, `raven config debug` shows resolved configuration with source annotations and `raven config validate` validates the config and warns on issues. Per PRD Section 5.11, these are listed under "Configuration commands." `config debug` is one of the three Phase 1 deliverables (PRD Section 7).

These commands wire together the entire configuration pipeline: find config file (T-009), load it (T-009), resolve with all layers (T-010), validate (T-011), and display results. They serve as the smoke test that the configuration system works end-to-end.

## Technical Specifications
### Implementation Approach
Create `internal/cli/config_cmd.go` with a parent `config` command and two subcommands: `debug` and `validate`. The `config` command itself has no action -- it serves as a namespace. `debug` loads and resolves the config, then prints each field with its value and source. `validate` loads, resolves, and validates the config, then prints errors and warnings.

### Key Components
- **configCmd**: Parent command (`raven config`) with no RunE
- **configDebugCmd**: `raven config debug` -- displays resolved config with sources
- **configValidateCmd**: `raven config validate` -- runs validation suite
- **formatResolvedConfig()**: Helper to format resolved config for display
- **formatValidationResult()**: Helper to format validation results for display

### API/Interface Contracts
```go
// internal/cli/config_cmd.go

package cli

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
    "github.com/charmbracelet/lipgloss"

    "<module>/internal/config"
)

var configCmd = &cobra.Command{
    Use:   "config",
    Short: "Configuration management commands",
    Long:  "Inspect, validate, and debug Raven configuration.",
}

var configDebugCmd = &cobra.Command{
    Use:   "debug",
    Short: "Show resolved configuration with source annotations",
    Long: `Display the fully-resolved configuration showing each value and
where it came from (cli flag, environment variable, config file, or default).`,
    Args: cobra.NoArgs,
    RunE: func(cmd *cobra.Command, args []string) error {
        resolved, err := loadAndResolveConfig()
        if err != nil {
            return err
        }
        printResolvedConfig(resolved)
        return nil
    },
}

var configValidateCmd = &cobra.Command{
    Use:   "validate",
    Short: "Validate configuration and report issues",
    Long:  "Check the configuration for errors and warnings.",
    Args:  cobra.NoArgs,
    RunE: func(cmd *cobra.Command, args []string) error {
        resolved, meta, err := loadAndResolveConfigWithMeta()
        if err != nil {
            return err
        }
        result := config.Validate(resolved.Config, meta)
        printValidationResult(result)
        if result.HasErrors() {
            return fmt.Errorf("configuration has %d error(s)", len(result.Errors()))
        }
        return nil
    },
}

func init() {
    configCmd.AddCommand(configDebugCmd)
    configCmd.AddCommand(configValidateCmd)
    rootCmd.AddCommand(configCmd)
}
```

**Example `raven config debug` output:**
```
Configuration Debug
===================

Config file: /Users/dev/myproject/raven.toml

[project]
  name         = "my-project"     (source: file)
  language     = "go"             (source: file)
  tasks_dir    = "docs/tasks"     (source: default)
  log_dir      = "/tmp/logs"      (source: env RAVEN_LOG_DIR)

[agents.claude]
  command      = "claude"         (source: file)
  model        = "claude-opus-4-6"  (source: cli --model)
  effort       = "high"           (source: file)

[review]
  extensions   = '(\.go$)'        (source: file)
```

**Example `raven config validate` output:**
```
Configuration Validation
========================

Errors:
  [project.name] must not be empty

Warnings:
  [project.log_dir] directory "scripts/logs" does not exist
  [unknown key] "project.taks_dir" is not a recognized field (did you mean "tasks_dir"?)

1 error(s), 2 warning(s)
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| github.com/spf13/cobra | v1.10+ | CLI framework |
| github.com/charmbracelet/lipgloss | v1.0+ | Colored output for sources and severity |
| internal/config | - | Config loading, resolution, validation (T-009, T-010, T-011) |

## Acceptance Criteria
- [ ] `raven config` shows help with available subcommands (debug, validate)
- [ ] `raven config debug` displays all resolved config fields with values and sources
- [ ] `raven config debug` shows the config file path used (or "none found")
- [ ] `raven config debug` works with no raven.toml (shows defaults only)
- [ ] `raven config debug` correctly shows CLI flag sources when flags are provided
- [ ] `raven config debug` correctly shows env var sources when RAVEN_* vars are set
- [ ] `raven config validate` with valid config prints "no errors" and exits 0
- [ ] `raven config validate` with invalid config prints errors and exits 1
- [ ] `raven config validate` prints warnings for unknown keys
- [ ] `raven config validate` prints warnings for non-existent directories
- [ ] Output uses color when terminal supports it (disabled with --no-color)
- [ ] Both commands respect --config flag to use a specific config file
- [ ] Both commands respect --dir flag for working directory
- [ ] `go vet ./...` passes
- [ ] Unit tests achieve 85% coverage

## Testing Requirements
### Unit Tests
- configDebugCmd with default config only: all sources show "default"
- configDebugCmd with config file: file fields show "file" source
- configValidateCmd with valid config: exits without error
- configValidateCmd with invalid config: returns error
- formatResolvedConfig produces expected output format
- formatValidationResult with errors shows error section
- formatValidationResult with warnings shows warning section
- formatValidationResult with no issues shows success message

### Integration Tests
- `raven config debug` against actual project raven.toml
- `raven config validate` against actual project raven.toml

### Edge Cases to Handle
- No raven.toml found: debug shows all defaults, validate validates defaults
- raven.toml is a directory (not a file): LoadFromFile returns error, command handles gracefully
- Very large config with many agents: output should be readable
- Config with special characters in string values: output should escape/quote properly

## Implementation Notes
### Recommended Approach
1. Create `internal/cli/config_cmd.go`
2. Define the parent `configCmd` with no RunE
3. Implement `configDebugCmd`:
   a. Call `config.FindConfigFile()` to locate raven.toml
   b. Call `config.LoadFromFile()` if found
   c. Call `config.Resolve()` with all layers
   d. Format and print each section with source annotations
4. Implement `configValidateCmd`:
   a. Load and resolve config (same as debug)
   b. Call `config.Validate()` with resolved config and TOML metadata
   c. Print errors and warnings with color coding
   d. Return error if validation has errors (exit code 1)
5. Create a shared `loadAndResolveConfig()` helper used by both commands
6. Use lipgloss for colored source labels: green for default, blue for file, yellow for env, red for cli
7. Register both subcommands under configCmd, register configCmd under rootCmd

### Potential Pitfalls
- The `loadAndResolveConfig()` helper must handle the case where no config file is found (use defaults only, meta is nil).
- The `--config` global flag should be checked first -- if set, use that path directly instead of auto-detection.
- The debug output format should be readable in both colored and non-colored modes. Use fixed-width alignment for the source annotation.
- Validate command should not short-circuit on the first error -- collect all issues and report them together.

### Security Considerations
- Debug output may show agent configuration details. This is acceptable for a local dev tool.
- Do not output environment variable values that might contain secrets (e.g., if someone puts an API key in RAVEN_* env var). Consider masking known-sensitive patterns.

## References
- [PRD Section 5.10 - raven config debug/validate](docs/prd/PRD-Raven.md)
- [PRD Section 5.11 - Configuration commands](docs/prd/PRD-Raven.md)
- [PRD Section 7 - Phase 1 Deliverables (config debug)](docs/prd/PRD-Raven.md)
- [Lipgloss Styling](https://github.com/charmbracelet/lipgloss)