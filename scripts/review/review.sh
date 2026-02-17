#!/usr/bin/env bash
# Raven multi-agent review orchestrator.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./review-lib.sh
source "$SCRIPT_DIR/review-lib.sh"

DRY_RUN=false
SKIP_REVIEW=false
SKIP_CONSOLIDATION=false
FILTER_AGENT=""
BASE_REF_INPUT="main"
MAX_CONCURRENCY=3
MODE=""

usage() {
  cat <<USAGE
Usage: $0 [OPTIONS]

Options:
  --agent <name>            Run only one agent (claude|codex|gemini)
  --skip-review             Skip review pass execution and reuse latest raw outputs
  --skip-consolidation      Skip consolidation phase
  --concurrency <n>         Max parallel review tasks (default: 3)
  --mode <none|agent|all>   Compatibility mode selector
  --base <ref>              Base git ref for diff (default: main)
  --description <text>      Extra author/change context for prompts
  --dry-run                 Print plan and commands without running agents
  -h, --help                Show this help
USAGE
}

selected_agents() {
  if [[ -n "$FILTER_AGENT" ]]; then
    echo "$FILTER_AGENT"
  else
    printf '%s\n' "${ALL_AGENTS[@]}"
  fi
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --agent)
        [[ $# -ge 2 ]] || die "--agent requires a value"
        FILTER_AGENT="$2"
        shift 2
        ;;
      --skip-review)
        SKIP_REVIEW=true
        shift
        ;;
      --skip-consolidation)
        SKIP_CONSOLIDATION=true
        shift
        ;;
      --concurrency)
        [[ $# -ge 2 ]] || die "--concurrency requires a value"
        MAX_CONCURRENCY="$2"
        shift 2
        ;;
      --mode)
        [[ $# -ge 2 ]] || die "--mode requires a value"
        MODE="$2"
        shift 2
        ;;
      --base)
        [[ $# -ge 2 ]] || die "--base requires a value"
        BASE_REF_INPUT="$2"
        shift 2
        ;;
      --description|--desc)
        [[ $# -ge 2 ]] || die "--description requires a value"
        CHANGE_DESCRIPTION="$2"
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
        die "Unknown argument: $1"
        usage
        exit 1
        ;;
    esac
  done

  if ! [[ "$MAX_CONCURRENCY" =~ ^[0-9]+$ ]] || (( MAX_CONCURRENCY < 1 )); then
    die "--concurrency must be a positive integer"
    exit 1
  fi

  if [[ -n "$FILTER_AGENT" ]] && ! is_valid_agent "$FILTER_AGENT"; then
    die "Invalid --agent '$FILTER_AGENT' (allowed: claude|codex|gemini)"
    exit 1
  fi

  if [[ -n "$MODE" ]]; then
    case "$MODE" in
      none)
        SKIP_REVIEW=true
        ;;
      agent)
        if [[ -z "$FILTER_AGENT" ]]; then
          die "--mode agent requires --agent <claude|codex|gemini>"
          exit 1
        fi
        ;;
      all)
        ;;
      *)
        die "Invalid --mode '$MODE' (allowed: none|agent|all)"
        exit 1
        ;;
    esac
  fi
}

preflight() {
  log_header "Preflight"

  local errors=0

  if ! assert_tool git; then
    errors=$((errors + 1))
  fi

  if ! assert_tool jq; then
    errors=$((errors + 1))
  else
    check_jq_version
  fi

  if command -v node >/dev/null 2>&1; then
    log_success "node -> $(command -v node)"
  else
    log_warn "node not found (JSON extraction may be less robust)"
  fi

  if [[ "$DRY_RUN" == "false" && "$SKIP_REVIEW" == "false" ]]; then
    local agent
    while IFS= read -r agent; do
      if ! assert_tool "$(agent_bin "$agent")"; then
        errors=$((errors + 1))
      fi
    done < <(selected_agents)
  fi

  if (( errors > 0 )); then
    die "Preflight failed"
    exit 1
  fi

  log_success "Preflight passed"
}

prepare_run() {
  if [[ "$SKIP_REVIEW" == "true" ]]; then
    if [[ "$DRY_RUN" == "true" ]]; then
      log_dim "[DRY-RUN] would reuse latest archived run for consolidation/report"
      return 0
    fi

    use_latest_review_run
    return 0
  fi

  if [[ "$DRY_RUN" == "true" ]]; then
    local resolved
    resolved="$(resolve_base_ref "$BASE_REF_INPUT" 2>/dev/null || true)"
    if [[ -n "$resolved" ]]; then
      log_dim "[DRY-RUN] resolved base ref: $resolved"
    else
      log_dim "[DRY-RUN] base ref '$BASE_REF_INPUT' will be resolved at runtime"
    fi
    return 0
  fi

  ensure_review_dirs
  collect_diff "$BASE_REF_INPUT"
}

run_reviews() {
  log_header "Review Passes"

  local tasks=()
  local agent
  local pass

  while IFS= read -r agent; do
    for pass in "${REVIEW_PASSES[@]}"; do
      tasks+=("$agent|$pass")
    done
  done < <(selected_agents)

  log_info "Tasks: ${#tasks[@]} (${MAX_CONCURRENCY} concurrent)"

  if [[ "$DRY_RUN" == "true" ]]; then
    local task
    for task in "${tasks[@]}"; do
      run_single_review "${task%%|*}" "${task##*|}" true
    done
    return 0
  fi

  local pids=()
  local task

  for task in "${tasks[@]}"; do
    while (( ${#pids[@]} >= MAX_CONCURRENCY )); do
      local remaining=()
      local pid
      for pid in "${pids[@]}"; do
        if kill -0 "$pid" >/dev/null 2>&1; then
          remaining+=("$pid")
        else
          wait "$pid" >/dev/null 2>&1 || true
        fi
      done
      pids=("${remaining[@]}")
      if (( ${#pids[@]} >= MAX_CONCURRENCY )); then
        sleep 1
      fi
    done

    run_single_review "${task%%|*}" "${task##*|}" false &
    pids+=("$!")
  done

  local pid
  for pid in "${pids[@]}"; do
    wait "$pid" >/dev/null 2>&1 || true
  done

  log_success "Review pass execution complete"
}

run_optional_consolidation() {
  if [[ "$SKIP_CONSOLIDATION" == "true" ]]; then
    log_info "Skipping consolidation (--skip-consolidation)"
    return 0
  fi

  log_header "Consolidation"
  run_consolidation "$DRY_RUN"

  if [[ "$DRY_RUN" == "false" ]]; then
    local consolidated_file="$RUN_DIR/consolidated.json"
    if [[ -f "$consolidated_file" ]]; then
      local verdict
      verdict="$(jq -r '.verdict // "COMMENT"' "$consolidated_file" 2>/dev/null || echo "COMMENT")"
      log_info "REVIEW_VERDICT: $verdict"
    fi
  fi
}

generate_outputs() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log_info "Dry-run complete"
    return 0
  fi

  if [[ "$SKIP_CONSOLIDATION" == "false" ]]; then
    generate_review_report
  fi

  if [[ "$SKIP_REVIEW" == "false" ]]; then
    archive_review_run
  fi

  print_run_paths
}

main() {
  parse_args "$@"

  echo
  echo "Raven Review"
  echo "============"

  preflight
  prepare_run

  if [[ "$SKIP_REVIEW" == "false" ]]; then
    run_reviews
  else
    log_info "Skipping review execution (--skip-review)"
  fi

  run_optional_consolidation
  generate_outputs
}

main "$@"
