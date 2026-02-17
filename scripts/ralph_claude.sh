#!/usr/bin/env bash
#
# ralph_claude.sh -- Ralph Wiggum Loop for Raven (Claude Code)
#
# Runs Claude Code in an autonomous loop, implementing one task per iteration.
# Each iteration gets a fresh context window. Memory persists via files on disk
# and git history.
#
# Usage:
#   ./scripts/ralph_claude.sh --phase 1                    # Run Phase 1 tasks
#   ./scripts/ralph_claude.sh --phase 2 --max-iterations 5
#   ./scripts/ralph_claude.sh --task T-003                  # Run single task
#   ./scripts/ralph_claude.sh --phase 1 --dry-run           # Preview prompt only
#   ./scripts/ralph_claude.sh --phase all                   # Run all phases sequentially
#   ./scripts/ralph_claude.sh --model claude-sonnet-4-5-20250929  # Use specific model
#
# Prerequisites:
#   - Claude Code CLI installed (`claude` command available)
#   - Git repository initialized
#   - docs/tasks/ populated with task specs and PROGRESS.md
#

set -euo pipefail

# =============================================================================
# Script-Specific Configuration
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PROMPT_TEMPLATE="$SCRIPT_DIR/RALPH-PROMPT.md"
LOG_DIR="$PROJECT_ROOT/scripts/logs"
PROGRESS_FILE="$PROJECT_ROOT/docs/tasks/PROGRESS.md"
TASK_STATE_FILE="$PROJECT_ROOT/docs/tasks/task-state.conf"

DEFAULT_MODEL="claude-opus-4-6"
DEFAULT_EFFORT="high"  # CLAUDE_CODE_EFFORT_LEVEL: low, medium, high
EFFORT_LABEL="Effort"
LOG_PREFIX="ralph-claude"
AGENT_LABEL="Claude Code"
AGENT_CMD="claude"

# =============================================================================
# Source Shared Library
# =============================================================================

source "$SCRIPT_DIR/ralph-lib.sh"

# =============================================================================
# Claude-Specific Allowed Tools
# =============================================================================

# Git subcommands are listed explicitly (no blanket "git *") to prevent
# accidental git push. The "git commit" pattern uses both bare and arg
# forms to work around Claude Code issue #1520 with complex commit messages.
CLAUDE_ALLOWED_TOOLS='Edit,Write,Read,Glob,Grep,Task,WebSearch,WebFetch,Bash(go build*),Bash(go test*),Bash(go vet*),Bash(go mod*),Bash(go get*),Bash(go run*),Bash(go fmt*),Bash(go install*),Bash(go version*),Bash(go generate*),Bash(git add *),Bash(git add),Bash(git commit *),Bash(git commit),Bash(git status*),Bash(git diff*),Bash(git log*),Bash(git rev-parse*),Bash(git stash*),Bash(git branch*),Bash(git checkout*),Bash(git merge*),Bash(git rebase*),Bash(git rm *),Bash(git mv *),Bash(git show*),Bash(git reset*),Bash(mkdir*),Bash(ls*),Bash(make*),Bash(chmod*),Bash(curl *),Bash(wget *),Bash(golangci-lint*),Bash(./bin/*),Bash(./scripts/*)'

# =============================================================================
# Agent-Specific Functions (required by ralph-lib.sh)
# =============================================================================

# Run Claude Code with a prompt file.
# Arguments: prompt_file, model, effort
# Outputs: agent stdout+stderr
run_agent() {
    local prompt_file="$1"
    local model="$2"
    local effort="$3"

    local args=()
    args+=(-p)
    args+=(--permission-mode dontAsk)
    if [[ -n "$model" ]]; then
        args+=(--model "$model")
    fi
    args+=(--allowedTools "$CLAUDE_ALLOWED_TOOLS")

    # Pass prompt via stdin to preserve content exactly and avoid eval/string command construction.
    claude "${args[@]}" < "$prompt_file" 2>&1
}

# Check that the claude CLI is available.
check_prerequisites() {
    if ! command -v claude &>/dev/null; then
        echo "ERROR: 'claude' CLI not found. Install Claude Code first."
        exit 1
    fi
}

# Pre-agent setup: export effort level for Claude Code.
pre_agent_setup() {
    local effort="$1"
    export CLAUDE_CODE_EFFORT_LEVEL="$effort"
}

# Return the dry-run command display string.
get_dry_run_command() {
    local model="$1"
    local effort="$2"
    echo "Claude command: CLAUDE_CODE_EFFORT_LEVEL=$effort cat <prompt> | claude -p --permission-mode dontAsk${model:+ --model $model} --allowedTools '...'"
}

# Script-specific usage text.
usage() {
    cat <<'EOF'
Ralph Wiggum Loop for Raven (Claude Code)

Usage:
  ./scripts/ralph_claude.sh --phase <phase>  [options]
  ./scripts/ralph_claude.sh --task <task-id> [options]
  ./scripts/ralph_claude.sh --status

Phases:
EOF
    phases_listing
    cat <<'EOF'

Options:
  --phase <id>           Phase to run (required unless --task)
  --task <T-XXX>         Run a single specific task
  --max-iterations <n>   Max loop iterations (default: 20)
  --max-limit-waits <n>  Max rate-limit wait cycles before abort (default: 2)
  --model <name>         Model to use (default: claude-opus-4-6)
  --effort <level>       Thinking effort: low, medium, high (default: high)
  --dry-run              Print generated prompt and model config without running
  --status               Show task completion status and exit
  -h, --help             Show this help

Examples:
  ./scripts/ralph_claude.sh --phase 1                        # Run all Phase 1 tasks
  ./scripts/ralph_claude.sh --phase 1 --model claude-sonnet-4-5-20250929  # Use Sonnet
  ./scripts/ralph_claude.sh --phase 1 --effort medium        # Lower thinking effort
  ./scripts/ralph_claude.sh --phase 1 --max-iterations 5     # Cap at 5 iterations
  ./scripts/ralph_claude.sh --phase 1 --max-limit-waits 3    # Allow 3 rate-limit waits
  ./scripts/ralph_claude.sh --task T-003                      # Run single task T-003
  ./scripts/ralph_claude.sh --phase 1 --dry-run               # Preview the prompt
  ./scripts/ralph_claude.sh --phase all                       # Run all phases sequentially
  ./scripts/ralph_claude.sh --status                          # Show completion status
EOF
}

# =============================================================================
# Entry Point
# =============================================================================

main "$@"
