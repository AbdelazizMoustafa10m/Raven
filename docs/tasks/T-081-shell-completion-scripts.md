# T-081: Shell Completion Installation Scripts and Packaging

## Metadata
| Field | Value |
|-------|-------|
| Priority | Should Have |
| Estimated Effort | Small: 3-4hrs |
| Dependencies | T-008, T-079 |
| Blocked By | T-008 |
| Blocks | T-087 |

## Goal
Create shell completion installation scripts for bash, zsh, fish, and PowerShell that users can run to set up tab completion for Raven. Additionally, configure GoReleaser to generate and package completion files as part of the release artifacts so they are available for download alongside the binary.

## Background
Per PRD Section 5.11, Raven provides shell completions via `raven completion <shell>` (bash, zsh, fish, powershell), implemented in T-008 using Cobra's built-in completion generation. This task creates the installer scripts that automate the process of generating and installing completions into the correct shell-specific directories, and integrates completion generation into the GoReleaser build so they ship with release archives.

Cobra generates completion scripts programmatically via `rootCmd.GenBashCompletionV2()`, `rootCmd.GenZshCompletion()`, `rootCmd.GenFishCompletion()`, and `rootCmd.GenPowerShellCompletion()`. The installation scripts wrap `raven completion <shell>` and copy the output to the appropriate system or user directory.

## Technical Specifications
### Implementation Approach
Create a `scripts/completions/` directory with installer scripts for each shell. Additionally, create a Go program at `scripts/gen-completions/main.go` that GoReleaser invokes as a `before.hook` to pre-generate all completion files into a `completions/` directory within the build artifact. Update `.goreleaser.yaml` to include completion files in the release archives.

### Key Components
- **`scripts/completions/install.sh`**: Universal installer script that detects the user's shell and installs completions
- **`scripts/gen-completions/main.go`**: Go program that generates all completion files for GoReleaser packaging
- **GoReleaser integration**: Extra files in archives + standalone completion archive
- **README documentation**: Installation instructions for each shell (covered in T-086)

### API/Interface Contracts
```bash
#!/usr/bin/env bash
# scripts/completions/install.sh
# Universal completion installer for Raven
#
# Usage:
#   ./install.sh              # Auto-detect shell and install
#   ./install.sh bash         # Install bash completions
#   ./install.sh zsh          # Install zsh completions
#   ./install.sh fish         # Install fish completions
#   RAVEN_BIN=./raven ./install.sh  # Use specific raven binary

set -euo pipefail

RAVEN_BIN="${RAVEN_BIN:-raven}"
SHELL_TYPE="${1:-}"

install_bash() {
    local target
    if [[ -d /usr/local/share/bash-completion/completions ]]; then
        target="/usr/local/share/bash-completion/completions/raven"
    elif [[ -d /etc/bash_completion.d ]]; then
        target="/etc/bash_completion.d/raven"
    else
        target="${HOME}/.local/share/bash-completion/completions/raven"
        mkdir -p "$(dirname "$target")"
    fi
    "$RAVEN_BIN" completion bash > "$target"
    echo "Bash completions installed to $target"
}

install_zsh() {
    local target="${HOME}/.zsh/completions/_raven"
    mkdir -p "$(dirname "$target")"
    "$RAVEN_BIN" completion zsh > "$target"
    echo "Zsh completions installed to $target"
    echo "Add to ~/.zshrc if not present: fpath=(~/.zsh/completions \$fpath)"
}

install_fish() {
    local target="${HOME}/.config/fish/completions/raven.fish"
    mkdir -p "$(dirname "$target")"
    "$RAVEN_BIN" completion fish > "$target"
    echo "Fish completions installed to $target"
}

# Shell detection and dispatch
if [[ -z "$SHELL_TYPE" ]]; then
    case "${SHELL:-}" in
        */bash) SHELL_TYPE="bash" ;;
        */zsh)  SHELL_TYPE="zsh" ;;
        */fish) SHELL_TYPE="fish" ;;
        *) echo "Cannot detect shell. Usage: $0 [bash|zsh|fish]"; exit 1 ;;
    esac
fi

case "$SHELL_TYPE" in
    bash) install_bash ;;
    zsh)  install_zsh ;;
    fish) install_fish ;;
    *) echo "Unsupported shell: $SHELL_TYPE"; exit 1 ;;
esac
```

```go
// scripts/gen-completions/main.go
package main

import (
	"fmt"
	"os"

	"raven/internal/cli"
)

func main() {
	outDir := "completions"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "creating output dir: %v\n", err)
		os.Exit(1)
	}

	rootCmd := cli.NewRootCmd()

	// Generate bash completions
	bashFile, _ := os.Create(outDir + "/raven.bash")
	rootCmd.GenBashCompletionV2(bashFile, true)
	bashFile.Close()

	// Generate zsh completions
	zshFile, _ := os.Create(outDir + "/_raven")
	rootCmd.GenZshCompletion(zshFile)
	zshFile.Close()

	// Generate fish completions
	fishFile, _ := os.Create(outDir + "/raven.fish")
	rootCmd.GenFishCompletion(fishFile, true)
	fishFile.Close()

	// Generate powershell completions
	psFile, _ := os.Create(outDir + "/raven.ps1")
	rootCmd.GenPowerShellCompletionWithDesc(psFile)
	psFile.Close()

	fmt.Printf("Completions generated in %s/\n", outDir)
}
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| spf13/cobra | v1.10+ | Built-in completion generation methods |
| internal/cli | - | Root command for completion generation |

## Acceptance Criteria
- [ ] `scripts/completions/install.sh` installs bash completions to the correct directory
- [ ] `scripts/completions/install.sh zsh` installs zsh completions with fpath instructions
- [ ] `scripts/completions/install.sh fish` installs fish completions to `~/.config/fish/completions/`
- [ ] `scripts/gen-completions/main.go` generates all four completion files (bash, zsh, fish, powershell)
- [ ] GoReleaser archives include the `completions/` directory with all completion files
- [ ] A standalone `raven_completions.tar.gz` artifact is published with each release
- [ ] Install script is idempotent (safe to run multiple times)
- [ ] Install script detects the user's shell when no argument is provided
- [ ] Install script works on both macOS and Linux

## Testing Requirements
### Unit Tests
- No Go unit tests for shell scripts
- Test `gen-completions/main.go` compiles and generates non-empty files for all four shells

### Integration Tests
- Run `install.sh bash` in a temporary HOME and verify the file is created
- Run `install.sh zsh` in a temporary HOME and verify the file is created at the expected path
- Run `install.sh fish` in a temporary HOME and verify the file is created at the expected path
- Verify generated bash completion file contains `_raven` function
- Verify generated zsh completion file contains `#compdef raven`
- Verify generated fish completion file contains `complete -c raven`

### Edge Cases to Handle
- User does not have write permission to system completion directories (fall back to user-local paths)
- `bash-completion` package not installed (completions directory may not exist)
- Fish shell not installed but user requests fish completions
- Running on macOS where `/etc/bash_completion.d/` may not exist
- Zsh fpath not configured to include user completions directory

## Implementation Notes
### Recommended Approach
1. Create `scripts/completions/install.sh` with the auto-detection and per-shell installation logic
2. Create `scripts/gen-completions/main.go` that imports the root command and generates all completion files
3. Update `.goreleaser.yaml` to include a `before.hook` that runs `go run scripts/gen-completions/main.go`
4. Update `.goreleaser.yaml` archives to include `completions/*` files
5. Test the install script on macOS (zsh default) and Linux (bash default)
6. Add `make completions` target to the Makefile

### Potential Pitfalls
- The `gen-completions/main.go` program must import the root command without side effects (no `init()` functions that require configuration files to exist)
- Cobra's `GenBashCompletionV2` requires a boolean parameter for including descriptions; use `true` for rich completions
- On macOS, the default shell is zsh but `$SHELL` might still report `/bin/bash` for users who have not updated
- Fish completions directory may not exist if fish is installed but never configured

### Security Considerations
- Install script should not use `sudo` by default; fall back to user-local directories
- Generated completion scripts are plain text and do not execute arbitrary code
- Ensure install script does not overwrite user customizations without warning

## References
- [Cobra Shell Completions Guide](https://cobra.dev/docs/how-to-guides/shell-completion/)
- [Shipping Completions with GoReleaser and Cobra](https://carlosbecker.com/posts/golang-completions-cobra/)
- [GoReleaser NFPM Completions Packaging](https://goreleaser.com/customization/nfpm/)
- [Bash Completion FAQ](https://github.com/scop/bash-completion/blob/master/README.md)