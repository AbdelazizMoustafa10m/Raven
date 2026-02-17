# T-006: Cobra CLI Root Command and Global Flags

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 4-8hrs |
| Dependencies | T-001, T-005 |
| Blocked By | T-001, T-005 |
| Blocks | T-007, T-008, T-012, T-014, T-047, T-055, T-078, T-082, T-084, T-086 |

## Goal
Create the Cobra root command with all global flags (`--verbose`, `--quiet`, `--config`, `--dir`, `--dry-run`, `--no-color`), global environment variable bindings, and the `PersistentPreRun` hook that initializes logging and working directory. This root command is the parent of all subcommands and defines Raven's CLI identity.

## Background
Per PRD Section 5.11, Raven's CLI uses `spf13/cobra` v1.10+ (same framework as `gh`, `kubectl`, `docker`). The root command defines six global flags, supports environment variable overrides with the `RAVEN_` prefix, and sets exit codes: 0 (success), 1 (error), 2 (partial success), 3 (user-cancelled). All progress/status goes to stderr; structured output goes to stdout.

Per PRD Section 6.5, the `--verbose` flag sets debug-level logging, `--quiet` suppresses all but errors, and `--no-color` disables colored output. These are wired up in the `PersistentPreRun` hook so they take effect before any subcommand executes.

Per CLAUDE.md, the root command is defined in `internal/cli/root.go` and `cmd/raven/main.go` simply calls `cli.Execute()`.

## Technical Specifications
### Implementation Approach
Create `internal/cli/root.go` with the root `cobra.Command` and an `Execute()` function called from `cmd/raven/main.go`. Define all global flags as persistent flags on the root command. Implement `PersistentPreRunE` to initialize logging (via T-005), resolve the working directory, and handle `--no-color`. Update `cmd/raven/main.go` to call `cli.Execute()` and exit with the appropriate code.

### Key Components
- **rootCmd**: The root `cobra.Command` with Use "raven", Short and Long descriptions
- **Global flags**: --verbose/-v, --quiet/-q, --config, --dir, --dry-run, --no-color
- **PersistentPreRunE**: Logging setup, working directory resolution, color detection
- **Execute()**: Public function that runs the root command and returns exit code
- **Exit code handling**: Map cobra errors to exit codes 0, 1, 2, 3

### API/Interface Contracts
```go
// internal/cli/root.go

// Package cli implements all Cobra commands for the Raven CLI.
package cli

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
    "github.com/charmbracelet/lipgloss"

    "<module>/internal/logging"
)

// Global flag values accessible to all subcommands.
var (
    flagVerbose bool
    flagQuiet   bool
    flagConfig  string
    flagDir     string
    flagDryRun  bool
    flagNoColor bool
)

// rootCmd is the base command for Raven.
var rootCmd = &cobra.Command{
    Use:   "raven",
    Short: "AI workflow orchestration command center",
    Long: `Raven is an AI workflow orchestration command center that manages the
full lifecycle of AI-assisted software development -- from PRD decomposition
to implementation, code review, fix application, and pull request creation.`,
    SilenceUsage:  true,
    SilenceErrors: true,
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        // Initialize logging
        jsonFormat := os.Getenv("RAVEN_LOG_FORMAT") == "json"
        logging.Setup(flagVerbose, flagQuiet, jsonFormat)

        // Handle --no-color
        if flagNoColor || os.Getenv("NO_COLOR") != "" || os.Getenv("RAVEN_NO_COLOR") != "" {
            lipgloss.SetColorProfile(lipgloss.Ascii)
        }

        // Handle --dir (change working directory)
        if flagDir != "" {
            if err := os.Chdir(flagDir); err != nil {
                return fmt.Errorf("changing directory to %s: %w", flagDir, err)
            }
        }

        return nil
    },
}

func init() {
    rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Enable verbose (debug) output")
    rootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "Suppress all output except errors")
    rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "Path to raven.toml config file")
    rootCmd.PersistentFlags().StringVar(&flagDir, "dir", "", "Override working directory")
    rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Show planned actions without executing")
    rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "Disable colored output")

    // Environment variable bindings
    rootCmd.PersistentFlags().Lookup("verbose").Usage += " (env: RAVEN_VERBOSE)"
    rootCmd.PersistentFlags().Lookup("quiet").Usage += " (env: RAVEN_QUIET)"
    rootCmd.PersistentFlags().Lookup("no-color").Usage += " (env: RAVEN_NO_COLOR, NO_COLOR)"
}

// Execute runs the root command and returns the exit code.
func Execute() int {
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        return 1
    }
    return 0
}
```

```go
// cmd/raven/main.go (updated)
package main

import (
    "os"
    "<module>/internal/cli"
)

func main() {
    os.Exit(cli.Execute())
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| github.com/spf13/cobra | v1.10+ | CLI framework |
| github.com/charmbracelet/lipgloss | v1.0+ | Color profile detection and --no-color |
| internal/logging | - | Logging setup (T-005) |

## Acceptance Criteria
- [ ] `internal/cli/root.go` exists with root command definition
- [ ] `cmd/raven/main.go` calls `cli.Execute()` and exits with returned code
- [ ] `raven --help` displays usage with all global flags
- [ ] `raven -v` / `--verbose` enables debug-level logging
- [ ] `raven -q` / `--quiet` suppresses all output except errors
- [ ] `raven --config /path/to/config.toml` stores the path for later resolution
- [ ] `raven --dir /tmp` changes the working directory before subcommand runs
- [ ] `raven --dry-run` flag is accessible to subcommands via `flagDryRun`
- [ ] `raven --no-color` disables colored output
- [ ] `NO_COLOR` and `RAVEN_NO_COLOR` environment variables disable color
- [ ] `RAVEN_LOG_FORMAT=json` enables JSON log format
- [ ] Running with no subcommand shows help (not an error)
- [ ] Unknown subcommand returns exit code 1 with error message to stderr
- [ ] `SilenceUsage` and `SilenceErrors` are set to true (Raven handles its own error output)
- [ ] `go build ./cmd/raven/` compiles successfully
- [ ] `go vet ./...` passes
- [ ] Unit tests achieve 85% coverage

## Testing Requirements
### Unit Tests
- Root command has correct Use, Short, and Long descriptions
- All six global flags are registered as persistent flags
- PersistentPreRunE with verbose=true calls logging.Setup with verbose=true
- PersistentPreRunE with quiet=true calls logging.Setup with quiet=true
- PersistentPreRunE with --dir to a valid directory changes CWD
- PersistentPreRunE with --dir to an invalid directory returns wrapped error
- Execute() returns 0 when no subcommand (help is displayed)
- Execute() returns 1 for unknown subcommand

### Integration Tests
- Built binary with `--help` flag exits 0 and prints to stdout
- Built binary with invalid `--dir` exits 1 and prints error to stderr

### Edge Cases to Handle
- Both `--verbose` and `--quiet` set: quiet wins (logging.Setup handles this per T-005)
- `--dir` with relative path: should resolve relative to current CWD
- `--dir` pointing to a file (not directory): os.Chdir returns error, handled gracefully
- `--config` with non-existent file: not validated here (validated in T-009/T-010)
- Running in a pipe (stdout not a terminal): lipgloss auto-detects and disables color

## Implementation Notes
### Recommended Approach
1. Create `internal/cli/root.go` with the root command and global flags
2. Implement `PersistentPreRunE` with logging setup and directory handling
3. Implement `Execute()` function with error-to-exit-code mapping
4. Update `cmd/raven/main.go` to call `cli.Execute()`
5. Create `internal/cli/root_test.go` testing flag registration and PreRun behavior
6. Verify: `go build ./cmd/raven/ && go vet ./... && ./dist/raven --help`

### Potential Pitfalls
- Cobra's `PersistentPreRunE` runs before ALL subcommands. Subcommands that define their own `PreRunE` will NOT automatically call the parent's `PersistentPreRunE` -- Cobra chains them correctly, but be aware of this if subcommands override `PersistentPreRunE`.
- `SilenceUsage: true` prevents Cobra from printing usage on errors. Raven provides its own error output.
- `SilenceErrors: true` prevents Cobra from printing errors. Raven handles error display in `Execute()`.
- Environment variable binding: Cobra does not natively bind env vars to flags. The env var values must be checked manually in `PersistentPreRunE` or use a helper to set flag defaults from env.
- `os.Chdir` affects the entire process. In tests, save and restore the original directory.

### Security Considerations
- `--config` flag accepts arbitrary file paths. The config loader (T-009) must validate file contents, not this flag.
- `--dir` allows changing to any accessible directory. This is expected behavior for a developer tool.
- Log output in verbose mode may include file paths and command arguments -- acceptable for a dev tool.

## References
- [Cobra User Guide](https://cobra.dev/)
- [Cobra v1.10 Release](https://github.com/spf13/cobra/releases)
- [spf13/cobra Documentation](https://pkg.go.dev/github.com/spf13/cobra)
- [PRD Section 5.11 - CLI Interface](docs/prd/PRD-Raven.md)
- [PRD Section 6.5 - Logging & Diagnostics](docs/prd/PRD-Raven.md)
- [Lipgloss Color Profile](https://github.com/charmbracelet/lipgloss#color-profiles)