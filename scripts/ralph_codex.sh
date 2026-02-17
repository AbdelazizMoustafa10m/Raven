#!/usr/bin/env bash
#
# ralph_codex.sh -- Ralph Wiggum Loop for Raven (Codex CLI)
#
# Runs OpenAI Codex CLI in an autonomous loop, implementing one task per iteration.
# Each iteration gets a fresh context window via `codex exec`. Memory persists via
# files on disk and git history.
#
# Usage:
#   ./scripts/ralph_codex.sh --phase 1                    # Run Phase 1 tasks
#   ./scripts/ralph_codex.sh --phase 2 --max-iterations 5
#   ./scripts/ralph_codex.sh --task T-003                  # Run single task
#   ./scripts/ralph_codex.sh --phase 1 --dry-run           # Preview prompt only
#   ./scripts/ralph_codex.sh --phase all                   # Run all phases sequentially
#   ./scripts/ralph_codex.sh --model o3                    # Use specific model
#
# Prerequisites:
#   - Codex CLI installed (`codex` command available)
#   - CODEX_API_KEY or authenticated via `codex login`
#   - Git repository initialized
#   - docs/tasks/ populated with task specs and PROGRESS.md
#

set -euo pipefail

# =============================================================================
# Script-Specific Configuration
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PROMPT_TEMPLATE="$SCRIPT_DIR/RALPH-PROMPT-CODEX.md"
LOG_DIR="$PROJECT_ROOT/scripts/logs"
PROGRESS_FILE="$PROJECT_ROOT/docs/tasks/PROGRESS.md"
TASK_STATE_FILE="$PROJECT_ROOT/docs/tasks/task-state.conf"

DEFAULT_MODEL="gpt-5.3-codex"
DEFAULT_EFFORT="high"  # model_reasoning_effort: minimal, low, medium, high, xhigh
EFFORT_LABEL="Reasoning"
LOG_PREFIX="ralph-codex"
AGENT_LABEL="Codex CLI"
AGENT_CMD="codex"

# =============================================================================
# Source Shared Library
# =============================================================================

source "$SCRIPT_DIR/ralph-lib.sh"

# =============================================================================
# Agent-Specific Functions (required by ralph-lib.sh)
# =============================================================================

# Run Codex CLI with a prompt file.
# Arguments: prompt_file, model, reasoning
# Outputs: agent stdout+stderr
run_agent() {
    local prompt_file="$1"
    local model="$2"
    local reasoning="$3"

    local args=()
    args+=(exec)
    args+=(--sandbox workspace-write)
    args+=(-a never)
    args+=(--ephemeral)

    if [[ -n "$model" ]]; then
        args+=(-m "$model")
    fi

    # Set reasoning effort level
    if [[ -n "$reasoning" ]]; then
        args+=(-c "model_reasoning_effort=\"$reasoning\"")
    fi

    # Pass '-' to force stdin prompt reading and avoid argv size/escaping issues.
    codex "${args[@]}" - < "$prompt_file" 2>&1
}

# Check that the codex CLI is available.
check_prerequisites() {
    if ! command -v codex &>/dev/null; then
        echo "ERROR: 'codex' CLI not found. Install Codex CLI first."
        echo ""
        echo "  npm install -g @openai/codex"
        echo ""
        echo "  Or see: https://developers.openai.com/codex/cli/"
        exit 1
    fi
}

# Pre-agent setup: no-op for Codex (no env vars needed).
pre_agent_setup() {
    : # Codex reasoning is passed via CLI args, not env vars
}

# Return the dry-run command display string.
get_dry_run_command() {
    local model="$1"
    local reasoning="$2"
    echo "Codex command: codex exec --sandbox workspace-write -a never --ephemeral${model:+ -m $model} -c model_reasoning_effort=\"$reasoning\" - < <prompt-file>"
}

# Script-specific usage text.
usage() {
    cat <<'EOF'
Ralph Wiggum Loop for Raven (Codex CLI)

Usage:
  ./scripts/ralph_codex.sh --phase <phase>  [options]
  ./scripts/ralph_codex.sh --task <task-id> [options]
  ./scripts/ralph_codex.sh --status

Phases:
EOF
    phases_listing
    cat <<'EOF'

Options:
  --phase <id>           Phase to run (required unless --task)
  --task <T-XXX>         Run a single specific task
  --max-iterations <n>   Max loop iterations (default: 20)
  --max-limit-waits <n>  Max rate-limit wait cycles before abort (default: 2)
  --model <name>         Model to use (default: gpt-5.3-codex)
  --reasoning <level>    Reasoning effort: minimal, low, medium, high, xhigh (default: high)
  --dry-run              Print generated prompt and model config without running
  --status               Show task completion status and exit
  -h, --help             Show this help

Examples:
  ./scripts/ralph_codex.sh --phase 1                        # Run all Phase 1 tasks
  ./scripts/ralph_codex.sh --phase 1 --model o3             # Use o3 model
  ./scripts/ralph_codex.sh --phase 1 --reasoning xhigh      # Max reasoning effort
  ./scripts/ralph_codex.sh --phase 1 --max-iterations 5     # Cap at 5 iterations
  ./scripts/ralph_codex.sh --phase 1 --max-limit-waits 3    # Allow 3 rate-limit waits
  ./scripts/ralph_codex.sh --task T-003                      # Run single task T-003
  ./scripts/ralph_codex.sh --phase 1 --dry-run               # Preview the prompt
  ./scripts/ralph_codex.sh --phase all                       # Run all phases sequentially
  ./scripts/ralph_codex.sh --status                          # Show completion status
EOF
}

# =============================================================================
# Entry Point
# =============================================================================

main "$@"
