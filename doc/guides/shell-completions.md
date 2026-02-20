# Shell Completions

Raven provides tab-completion scripts for bash, zsh, fish, and PowerShell. This page covers generation and installation for each shell.

## Bash

```bash
# Generate and install for the current user (Linux)
mkdir -p ~/.local/share/bash-completion/completions
raven completion bash > ~/.local/share/bash-completion/completions/raven

# macOS with bash-completion@2 via Homebrew
raven completion bash > $(brew --prefix)/etc/bash_completion.d/raven
```

## Zsh

```zsh
# Add to ~/.zshrc if $fpath is already set up:
raven completion zsh > "${fpath[1]}/_raven"

# Or generate to a completions directory:
mkdir -p ~/.zsh/completions
raven completion zsh > ~/.zsh/completions/_raven
# Add to ~/.zshrc: fpath=(~/.zsh/completions $fpath)
# Then: autoload -Uz compinit && compinit
```

## Fish

```fish
raven completion fish > ~/.config/fish/completions/raven.fish
```

## PowerShell

```powershell
raven completion powershell | Out-String | Invoke-Expression
# To persist, add the above line to your $PROFILE.
```

## Install Script

The release archive includes an install script that auto-detects your shell:

```bash
./scripts/completions/install.sh
```

## Man Pages

Generate and install man pages (requires the release binary or a source build):

```bash
# Generate to man/man1/
go run ./scripts/gen-manpages man/man1

# Install system-wide (requires sudo)
sudo ./scripts/install-manpages.sh

# View a man page
man raven-implement
```

Man pages are also included in the release archive at `man/man1/`.
