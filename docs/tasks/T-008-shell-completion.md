# T-008: Shell Completion Command -- raven completion

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 2-3hrs |
| Dependencies | T-006 |
| Blocked By | T-006 |
| Blocks | T-081 |

## Goal
Implement the `raven completion <shell>` command that generates shell completion scripts for bash, zsh, fish, and PowerShell. This enables tab completion for all Raven commands, flags, and arguments, improving developer ergonomics. The generated completions are consumed by T-081 (shell completion installation scripts).

## Background
Per PRD Section 5.11, `raven completion <shell>` generates shell completions for bash, zsh, fish, and powershell. Cobra provides built-in completion generation via `GenBashCompletionV2()`, `GenZshCompletion()`, `GenFishCompletion()`, and `GenPowerShellCompletion()`. These generate scripts that source dynamic completion logic -- as new subcommands and flags are added in later tasks, completions automatically pick them up.

Cobra's default completion command (`cobra.Command.CompletionOptions`) can be customized, but for Raven we create a custom completion command to control the output format, help text, and shell detection.

## Technical Specifications
### Implementation Approach
Create `internal/cli/completion.go` with a Cobra command that accepts a positional argument for the shell name (bash, zsh, fish, powershell). Use Cobra's built-in generation methods to write the completion script to stdout. Include usage examples in the help text showing how to install completions for each shell.

### Key Components
- **completionCmd**: Cobra command for `raven completion <shell>`
- **Shell-specific generation**: Delegates to Cobra's GenBashCompletionV2, GenZshCompletion, GenFishCompletion, GenPowerShellCompletion
- **ValidArgs**: Limits positional arg to bash, zsh, fish, powershell

### API/Interface Contracts
```go
// internal/cli/completion.go

package cli

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
    Use:       "completion [bash|zsh|fish|powershell]",
    Short:     "Generate shell completion scripts",
    Long: `Generate shell completion scripts for Raven.

To install completions:

  Bash (Linux):
    raven completion bash | sudo tee /etc/bash_completion.d/raven > /dev/null

  Bash (macOS with Homebrew):
    raven completion bash > $(brew --prefix)/etc/bash_completion.d/raven

  Zsh:
    raven completion zsh > "${fpath[1]}/_raven"
    # or
    raven completion zsh > ~/.zsh/completions/_raven

  Fish:
    raven completion fish > ~/.config/fish/completions/raven.fish

  PowerShell:
    raven completion powershell > raven.ps1
    # Then add ". raven.ps1" to your PowerShell profile`,
    DisableFlagsInUseLine: true,
    ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
    Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
    RunE: func(cmd *cobra.Command, args []string) error {
        switch args[0] {
        case "bash":
            return rootCmd.GenBashCompletionV2(os.Stdout, true)
        case "zsh":
            return rootCmd.GenZshCompletion(os.Stdout)
        case "fish":
            return rootCmd.GenFishCompletion(os.Stdout, true)
        case "powershell":
            return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
        default:
            return fmt.Errorf("unsupported shell: %s", args[0])
        }
    },
}

func init() {
    rootCmd.AddCommand(completionCmd)
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| github.com/spf13/cobra | v1.10+ | Built-in completion generation |

## Acceptance Criteria
- [ ] `raven completion bash` outputs a valid bash completion script to stdout
- [ ] `raven completion zsh` outputs a valid zsh completion script to stdout
- [ ] `raven completion fish` outputs a valid fish completion script to stdout
- [ ] `raven completion powershell` outputs a valid PowerShell completion script to stdout
- [ ] Missing shell argument produces error with usage hint
- [ ] Invalid shell name (e.g., "ksh") produces error
- [ ] Extra arguments produce error (exactly 1 required)
- [ ] Completion scripts include descriptions (pass `true` to V2 and Fish generators)
- [ ] Command help text includes installation examples for all four shells
- [ ] Command is listed in `raven --help` output
- [ ] Exit code is 0 on success, 1 on error
- [ ] `go vet ./...` passes
- [ ] Unit tests achieve 90% coverage

## Testing Requirements
### Unit Tests
- completionCmd with "bash" produces non-empty output containing "bash"
- completionCmd with "zsh" produces non-empty output containing "zsh" or "_raven"
- completionCmd with "fish" produces non-empty output containing "fish" or "complete"
- completionCmd with "powershell" produces non-empty output containing "PowerShell" or "Register"
- completionCmd with no arguments returns error
- completionCmd with invalid shell returns error
- completionCmd with two arguments returns error

### Integration Tests
- `raven completion bash | bash -n` validates bash syntax (if bash available)
- `raven completion zsh` outputs valid script (no syntax validation needed, Cobra generates valid zsh)

### Edge Cases to Handle
- Shell name with mixed case ("Bash" vs "bash"): currently case-sensitive, reject non-lowercase
- Output redirection: must work when stdout is a pipe (no terminal detection needed)
- Very long completion descriptions: Cobra handles truncation

## Implementation Notes
### Recommended Approach
1. Create `internal/cli/completion.go`
2. Define `completionCmd` with `ValidArgs` and `ExactArgs(1)`
3. Implement `RunE` with switch on shell name
4. Use `GenBashCompletionV2` (not V1) for modern bash completions with descriptions
5. Use `GenPowerShellCompletionWithDesc` for PowerShell completions with descriptions
6. Register via `rootCmd.AddCommand(completionCmd)` in `init()`
7. Create `internal/cli/completion_test.go`
8. Test: `make build && ./dist/raven completion bash | head -5`

### Potential Pitfalls
- Use `GenBashCompletionV2` (not `GenBashCompletion`) -- V2 supports descriptions and uses Cobra's Go-based dynamic completion system. V1 uses the legacy bash functions.
- Use `GenPowerShellCompletionWithDesc` (not `GenPowerShellCompletion`) for description support.
- The generated scripts call back into the `raven` binary for dynamic completions (via the hidden `__complete` command that Cobra registers automatically). The binary must be in PATH for this to work.
- Do not disable Cobra's default completion handling (`rootCmd.CompletionOptions.DisableDefaultCmd = true`) -- the hidden `__complete` command is needed for dynamic completions.

### Security Considerations
- Completion scripts are generated by Cobra and are safe to source. They do not execute arbitrary code.
- The generated scripts do not contain any project-specific secrets.

## References
- [Cobra Shell Completions Guide](https://cobra.dev/docs/how-to-guides/shell-completion/)
- [Cobra GenBashCompletionV2](https://pkg.go.dev/github.com/spf13/cobra#Command.GenBashCompletionV2)
- [PRD Section 5.11 - CLI Interface (completion command)](docs/prd/PRD-Raven.md)
- [PRD Section 7 - Phase 1: Shell completions](docs/prd/PRD-Raven.md)