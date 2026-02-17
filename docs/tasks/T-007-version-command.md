# T-007: Version Command -- raven version

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 1-2hrs |
| Dependencies | T-003, T-006 |
| Blocked By | T-003, T-006 |
| Blocks | None |

## Goal
Implement the `raven version` command that displays build information (version, commit, date) in both human-readable and JSON formats. This is one of the three Phase 1 deliverables and validates that the build info injection pipeline (ldflags -> buildinfo -> CLI) works end-to-end.

## Background
Per PRD Section 5.11, `raven version [--json]` shows version and build info. This command is listed as a Phase 1 deliverable (PRD Section 7). It consumes the `buildinfo` package (T-003) and registers as a subcommand of the root command (T-006). The `--json` flag enables structured output to stdout for scripting/CI consumption, following the PRD convention that structured output goes to stdout.

## Technical Specifications
### Implementation Approach
Create `internal/cli/version.go` with a Cobra command that reads from `buildinfo.GetInfo()` and prints either a human-readable string or JSON to stdout. Register it in the root command's `init()`.

### Key Components
- **versionCmd**: Cobra command for `raven version`
- **--json flag**: Outputs structured JSON instead of human-readable text
- **Output**: Human-readable to stdout; JSON to stdout (both are structured output per PRD convention)

### API/Interface Contracts
```go
// internal/cli/version.go

package cli

import (
    "encoding/json"
    "fmt"
    "os"

    "github.com/spf13/cobra"
    "<module>/internal/buildinfo"
)

var versionJSON bool

var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Show Raven version and build information",
    Long:  "Display the version, git commit, and build date of this Raven binary.",
    Args:  cobra.NoArgs,
    RunE: func(cmd *cobra.Command, args []string) error {
        info := buildinfo.GetInfo()

        if versionJSON {
            enc := json.NewEncoder(os.Stdout)
            enc.SetIndent("", "  ")
            return enc.Encode(info)
        }

        fmt.Println(info.String())
        return nil
    },
}

func init() {
    versionCmd.Flags().BoolVar(&versionJSON, "json", false, "Output version info as JSON")
    rootCmd.AddCommand(versionCmd)
}
```

**Human-readable output:**
```
raven v2.0.0 (commit: a1b2c3d, built: 2026-02-17T10:00:00Z)
```

**JSON output (`--json`):**
```json
{
  "version": "2.0.0",
  "commit": "a1b2c3d",
  "date": "2026-02-17T10:00:00Z"
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| encoding/json | stdlib | JSON output formatting |
| github.com/spf13/cobra | v1.10+ | CLI framework |
| internal/buildinfo | - | Build information (T-003) |

## Acceptance Criteria
- [ ] `raven version` prints the human-readable version string to stdout
- [ ] `raven version --json` prints indented JSON to stdout
- [ ] JSON output contains "version", "commit", and "date" fields
- [ ] Exit code is 0 on success
- [ ] `raven version` with no ldflags shows defaults: "dev", "unknown", "unknown"
- [ ] `raven version --json | jq .version` returns the version string (valid JSON)
- [ ] `raven version` accepts no positional arguments (extra args produce error)
- [ ] Command is listed in `raven --help` output
- [ ] `go vet ./...` passes
- [ ] Unit tests achieve 95% coverage

## Testing Requirements
### Unit Tests
- versionCmd with no flags produces human-readable output containing "raven v"
- versionCmd with --json flag produces valid JSON with version/commit/date keys
- versionCmd rejects extra arguments (cobra.NoArgs)
- Output goes to stdout (not stderr)

### Integration Tests
- Built binary: `./raven version` exits 0 and outputs version string
- Built binary with ldflags: `./raven version --json` outputs injected version

### Edge Cases to Handle
- Default version "dev" when built without ldflags (should not panic)
- `--json` with piped output (no terminal): should still produce JSON
- Very long version strings from `git describe --dirty` with many commits since tag

## Implementation Notes
### Recommended Approach
1. Create `internal/cli/version.go`
2. Define `versionCmd` with `cobra.NoArgs` for validation
3. Implement `RunE` with `--json` flag check
4. Register via `rootCmd.AddCommand(versionCmd)` in `init()`
5. Create `internal/cli/version_test.go`
6. Test end-to-end: `make build && ./dist/raven version`

### Potential Pitfalls
- Use `json.NewEncoder(os.Stdout)` rather than `json.Marshal` + `fmt.Println` to avoid double newline
- Use `SetIndent("", "  ")` for pretty JSON; piped consumers can handle whitespace
- Do not use `cmd.Println` for the version output -- it respects Cobra's output writer which may not be stdout in all cases. Use `fmt.Println` or `os.Stdout` directly.

### Security Considerations
- Version information is non-sensitive; safe to expose
- JSON output should not include any fields beyond version/commit/date

## References
- [Cobra Command Documentation](https://pkg.go.dev/github.com/spf13/cobra#Command)
- [PRD Section 5.11 - CLI Interface (version command)](docs/prd/PRD-Raven.md)
- [PRD Section 7 - Phase 1 Deliverables](docs/prd/PRD-Raven.md)