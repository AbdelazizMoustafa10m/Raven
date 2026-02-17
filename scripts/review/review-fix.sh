#!/usr/bin/env bash
# Apply fixes from consolidated review findings using a selected agent.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./review-lib.sh
source "$SCRIPT_DIR/review-lib.sh"

AGENT="codex"
RUN_INPUT=""
CONSOLIDATED_FILE=""
COMMIT_MESSAGE="chore(review): apply consolidated review fixes"
DRY_RUN=false
FIX_DESCRIPTION=""

usage() {
  cat <<USAGE
Usage: $0 [OPTIONS]

Options:
  --agent <name>           Fixer agent (claude|codex|gemini), default: codex
  --run <path>             Review run dir or consolidated.json path (default: reports/review/latest)
  --description <text>     Extra context for the fixer agent
  --commit-message <text>  Commit message (default: chore(review): apply consolidated review fixes)
  --dry-run                Print plan and command only
  -h, --help               Show this help
USAGE
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --agent)
        [[ $# -ge 2 ]] || { echo "RALPH_ERROR: --agent requires a value"; exit 1; }
        AGENT="$2"
        shift 2
        ;;
      --run)
        [[ $# -ge 2 ]] || { echo "RALPH_ERROR: --run requires a value"; exit 1; }
        RUN_INPUT="$2"
        shift 2
        ;;
      --description|--desc)
        [[ $# -ge 2 ]] || { echo "RALPH_ERROR: --description requires a value"; exit 1; }
        FIX_DESCRIPTION="$2"
        shift 2
        ;;
      --commit-message)
        [[ $# -ge 2 ]] || { echo "RALPH_ERROR: --commit-message requires a value"; exit 1; }
        COMMIT_MESSAGE="$2"
        shift 2
        ;;
      --dry-run)
        DRY_RUN=true
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "RALPH_ERROR: unknown argument '$1'"
        usage
        exit 1
        ;;
    esac
  done

  if ! is_valid_agent "$AGENT"; then
    echo "RALPH_ERROR: invalid agent '$AGENT' (allowed: claude|codex|gemini)"
    exit 1
  fi
}

resolve_consolidated_file() {
  if [[ -n "$RUN_INPUT" ]]; then
    if [[ -d "$RUN_INPUT" ]]; then
      RUN_DIR="$(cd "$RUN_INPUT" && pwd)"
      CONSOLIDATED_FILE="$RUN_DIR/consolidated.json"
      RAW_DIR="$RUN_DIR/raw"
      REVIEW_USING_ARCHIVE=true
      return 0
    fi

    if [[ -f "$RUN_INPUT" ]]; then
      CONSOLIDATED_FILE="$(cd "$(dirname "$RUN_INPUT")" && pwd)/$(basename "$RUN_INPUT")"
      RUN_DIR="$(cd "$(dirname "$CONSOLIDATED_FILE")" && pwd)"
      RAW_DIR="$RUN_DIR/raw"
      REVIEW_USING_ARCHIVE=true
      return 0
    fi

    echo "RALPH_ERROR: --run path not found: $RUN_INPUT"
    exit 1
  fi

  if [[ "$DRY_RUN" == "true" ]]; then
    CONSOLIDATED_FILE="$REPORTS_BASE/latest/consolidated.json"
    return 0
  fi

  use_latest_review_run
  CONSOLIDATED_FILE="$RUN_DIR/consolidated.json"
}

preflight() {
  log_header "Fix Preflight"

  assert_tool git >/dev/null || { echo "RALPH_ERROR: git not found"; exit 1; }
  assert_tool jq >/dev/null || { echo "RALPH_ERROR: jq not found"; exit 1; }

  if [[ "$DRY_RUN" == "false" ]]; then
    if ! command -v "$(agent_bin "$AGENT")" >/dev/null 2>&1; then
      echo "RALPH_ERROR: fixer agent binary not found: $(agent_bin "$AGENT")"
      exit 1
    fi

    if ! git -C "$PROJECT_ROOT" diff --quiet >/dev/null 2>&1 || ! git -C "$PROJECT_ROOT" diff --cached --quiet >/dev/null 2>&1; then
      echo "RALPH_ERROR: git working tree not clean before review-fix"
      exit 1
    fi
  fi

  if [[ "$DRY_RUN" == "false" && ! -f "$CONSOLIDATED_FILE" ]]; then
    echo "RALPH_ERROR: consolidated findings missing at $CONSOLIDATED_FILE"
    exit 1
  fi
}

build_fix_prompt() {
  local model
  model="$(agent_model "$AGENT")"

  cat <<PROMPT
You are a senior Go engineer applying review fixes for Raven.

Target repository: $PROJECT_ROOT
Agent label: $AGENT
Requested model: $model

Read consolidated findings from:
$CONSOLIDATED_FILE

Instructions:
1. Read and prioritize findings by severity: critical -> high -> medium -> low -> suggestion.
2. Implement concrete code fixes for valid findings.
3. Preserve Raven conventions from AGENTS.md (Go/Cobra standards, deterministic behavior, no panics, error wrapping, tests where needed).
4. Keep edits focused; avoid unrelated refactors.
5. Do NOT run git commit yourself. The wrapper script will run verification and commit.

Additional context from caller:
$FIX_DESCRIPTION

When done, print a short plaintext summary of files changed and findings addressed.
PROMPT
}

run_fixer_agent() {
  local prompt="$1"
  local output_file="$2"
  local log_file="$3"
  local model
  model="$(agent_model "$AGENT")"

  case "$AGENT" in
    claude)
      claude -p "$prompt" --model "$model" > "$output_file" 2> "$log_file"
      ;;
    codex)
      codex exec "$prompt" -m "$model" > "$output_file" 2> "$log_file"
      ;;
    gemini)
      gemini -p "$prompt" -m "$model" > "$output_file" 2> "$log_file"
      ;;
    *)
      return 1
      ;;
  esac
}

run_verification() {
  local verification_log="$1"

  local -a commands=(
    "go build ./cmd/raven/"
    "go vet ./..."
    "go test ./..."
    "go mod tidy"
  )

  local cmd
  for cmd in "${commands[@]}"; do
    log_info "Verify: $cmd"
    # Split command string into array for safe execution (no eval)
    local -a cmd_parts
    read -ra cmd_parts <<< "$cmd"
    if ! (cd "$PROJECT_ROOT" && "${cmd_parts[@]}") >> "$verification_log" 2>&1; then
      echo "RALPH_ERROR: verification failed: $cmd"
      return 1
    fi
  done

  log_success "Verification commands passed"
  return 0
}

commit_changes() {
  local before_head="$1"
  local commit_log="$2"

  (cd "$PROJECT_ROOT" && git add -A)

  if (cd "$PROJECT_ROOT" && git diff --cached --quiet); then
    echo "RALPH_ERROR: commit missing for review-fix (no staged changes)"
    return 1
  fi

  if ! (cd "$PROJECT_ROOT" && git commit -m "$COMMIT_MESSAGE") >> "$commit_log" 2>&1; then
    echo "RALPH_ERROR: commit missing for review-fix (git commit failed)"
    return 1
  fi

  local after_head
  after_head="$(git -C "$PROJECT_ROOT" rev-parse HEAD)"
  if [[ "$before_head" == "$after_head" ]]; then
    echo "RALPH_ERROR: commit missing for review-fix (HEAD unchanged)"
    return 1
  fi

  log_success "Committed fixes: $after_head"
  return 0
}

main() {
  parse_args "$@"
  resolve_consolidated_file
  preflight

  local fix_output_file
  local fix_log_file
  local verify_log_file
  local commit_log_file
  local run_stamp
  run_stamp="$(date '+%Y%m%d-%H%M%S')"

  if [[ -z "${RUN_DIR:-}" ]]; then
    RUN_DIR="$PROJECT_ROOT/reports/review"
  fi

  mkdir -p "$RUN_DIR"

  fix_output_file="$RUN_DIR/fix-${AGENT}-${run_stamp}.txt"
  fix_log_file="$RUN_DIR/fix-${AGENT}-${run_stamp}.log"
  verify_log_file="$RUN_DIR/fix-verify-${run_stamp}.log"
  commit_log_file="$RUN_DIR/fix-commit-${run_stamp}.log"

  local prompt
  prompt="$(build_fix_prompt)"

  log_header "Review Fix"
  log_info "Agent: $AGENT"
  log_info "Consolidated findings: $CONSOLIDATED_FILE"

  if [[ "$DRY_RUN" == "true" ]]; then
    log_dim "[DRY-RUN] agent command: $(agent_bin "$AGENT") (model: $(agent_model "$AGENT"))"
    log_dim "[DRY-RUN] verification commands: go build ./cmd/raven/ && go vet ./... && go test ./... && go mod tidy"
    log_dim "[DRY-RUN] commit message: $COMMIT_MESSAGE"
    exit 0
  fi

  local before_head
  before_head="$(git -C "$PROJECT_ROOT" rev-parse HEAD)"

  if ! run_fixer_agent "$prompt" "$fix_output_file" "$fix_log_file"; then
    echo "RALPH_ERROR: fixer agent failed ($AGENT). See $fix_log_file"
    exit 1
  fi

  run_verification "$verify_log_file" || exit 1
  commit_changes "$before_head" "$commit_log_file" || exit 1

  log_success "Review-fix completed"
  log_info "Agent output: $fix_output_file"
  log_info "Verification log: $verify_log_file"
  log_info "Commit log: $commit_log_file"
}

main "$@"
