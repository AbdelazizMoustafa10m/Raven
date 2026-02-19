#!/usr/bin/env bash
# scripts/completions/install.sh
# Universal completion installer for Raven
#
# Usage:
#   ./install.sh              # Auto-detect shell and install
#   ./install.sh bash         # Install bash completions
#   ./install.sh zsh          # Install zsh completions
#   ./install.sh fish         # Install fish completions
#   ./install.sh powershell   # Install PowerShell completions
#   RAVEN_BIN=./raven ./install.sh  # Use specific raven binary

set -euo pipefail

RAVEN_BIN="${RAVEN_BIN:-raven}"
SHELL_TYPE="${1:-}"

# Verify the raven binary is accessible.
if ! command -v "$RAVEN_BIN" > /dev/null 2>&1 && [[ ! -x "$RAVEN_BIN" ]]; then
    echo "Error: raven binary not found at '$RAVEN_BIN'." >&2
    echo "Set RAVEN_BIN to the path of your raven binary, e.g.:" >&2
    echo "  RAVEN_BIN=/usr/local/bin/raven $0 $*" >&2
    exit 1
fi

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
    echo "Reload your shell or run: source $target"
}

install_zsh() {
    local target="${HOME}/.zsh/completions/_raven"
    mkdir -p "$(dirname "$target")"
    "$RAVEN_BIN" completion zsh > "$target"
    echo "Zsh completions installed to $target"
    echo "Add to ~/.zshrc if not present: fpath=(~/.zsh/completions \$fpath)"
    echo "Then run: autoload -Uz compinit && compinit"
}

install_fish() {
    local target="${HOME}/.config/fish/completions/raven.fish"
    mkdir -p "$(dirname "$target")"
    "$RAVEN_BIN" completion fish > "$target"
    echo "Fish completions installed to $target"
}

install_powershell() {
    # Determine the PowerShell profile directory. On Windows (Git Bash / WSL)
    # the profile lives under the user's Documents folder. On Linux/macOS with
    # pwsh installed, $HOME/.config/powershell is the standard location.
    local profile_dir
    if [[ -n "${USERPROFILE:-}" ]]; then
        # Windows (Git Bash, MSYS2, Cygwin).
        profile_dir="${USERPROFILE}/Documents/PowerShell/Modules/RavenCompletion"
    else
        # Linux / macOS with pwsh.
        profile_dir="${HOME}/.config/powershell/Modules/RavenCompletion"
    fi

    mkdir -p "$profile_dir"
    local target="${profile_dir}/raven.ps1"
    "$RAVEN_BIN" completion powershell > "$target"
    echo "PowerShell completions installed to $target"
    echo ""
    echo "To load completions in every PowerShell session, add this line to your"
    echo "PowerShell profile (e.g., \$PROFILE):"
    echo "  . \"$target\""
}

# Shell detection and dispatch
if [[ -z "$SHELL_TYPE" ]]; then
    case "${SHELL:-}" in
        */bash) SHELL_TYPE="bash" ;;
        */zsh)  SHELL_TYPE="zsh" ;;
        */fish) SHELL_TYPE="fish" ;;
        */pwsh|*/powershell) SHELL_TYPE="powershell" ;;
        *)
            echo "Cannot detect shell. Usage: $0 [bash|zsh|fish|powershell]" >&2
            exit 1
            ;;
    esac
fi

case "$SHELL_TYPE" in
    bash)       install_bash ;;
    zsh)        install_zsh ;;
    fish)       install_fish ;;
    powershell|pwsh) install_powershell ;;
    *)
        echo "Unsupported shell: $SHELL_TYPE" >&2
        echo "Supported shells: bash, zsh, fish, powershell" >&2
        exit 1
        ;;
esac
