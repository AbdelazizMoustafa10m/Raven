#!/usr/bin/env bash
#
# task-state.sh -- Manage docs/tasks/task-state.conf
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TASK_STATE_FILE="${TASK_STATE_FILE:-$PROJECT_ROOT/docs/tasks/task-state.conf}"
PROGRESS_FILE="${PROGRESS_FILE:-$PROJECT_ROOT/docs/tasks/PROGRESS.md}"

source "$SCRIPT_DIR/phases-lib.sh"
source "$SCRIPT_DIR/task-state-lib.sh"

usage() {
    cat <<'EOF'
Usage:
  ./scripts/task-state.sh init
  ./scripts/task-state.sh bootstrap
  ./scripts/task-state.sh sync-from-progress
  ./scripts/task-state.sh get <T-XXX>
  ./scripts/task-state.sh is-complete <T-XXX>
  ./scripts/task-state.sh set <T-XXX> <completed|not_started|in_progress|blocked>
  ./scripts/task-state.sh count-remaining <T-XXX:T-YYY>
  ./scripts/task-state.sh summary

Commands:
  init               Reset task-state.conf to all not_started.
  bootstrap          Init then sync from PROGRESS.md (phase/task statuses).
  sync-from-progress Apply statuses from PROGRESS.md onto task-state.conf.
  get                Print normalized status for one task.
  is-complete        Exit 0 if task is completed, 1 otherwise.
  set                Update one task status with today's date stamp.
  count-remaining    Print remaining task count for a range.
  summary            Print status counts.
EOF
}

cmd="${1:-}"
if [[ -z "$cmd" ]]; then
    usage
    exit 1
fi
shift || true

case "$cmd" in
    init)
        task_state_init_file "$TASK_STATE_FILE"
        echo "Initialized $TASK_STATE_FILE"
        ;;
    bootstrap)
        task_state_init_file "$TASK_STATE_FILE"
        task_state_sync_from_progress "$PROGRESS_FILE" "$TASK_STATE_FILE"
        echo "Bootstrapped $TASK_STATE_FILE from $PROGRESS_FILE"
        ;;
    sync-from-progress)
        task_state_ensure_file "$PROGRESS_FILE" "$TASK_STATE_FILE"
        task_state_sync_from_progress "$PROGRESS_FILE" "$TASK_STATE_FILE"
        echo "Synced $TASK_STATE_FILE from $PROGRESS_FILE"
        ;;
    get)
        task_id="${1:-}"
        if [[ -z "$task_id" ]]; then
            usage
            exit 1
        fi
        task_state_get_status "$task_id" "$TASK_STATE_FILE"
        ;;
    is-complete)
        task_id="${1:-}"
        if [[ -z "$task_id" ]]; then
            usage
            exit 1
        fi
        if task_state_is_completed "$task_id" "$TASK_STATE_FILE"; then
            echo "yes"
            exit 0
        fi
        echo "no"
        exit 1
        ;;
    set)
        task_id="${1:-}"
        status="${2:-}"
        if [[ -z "$task_id" || -z "$status" ]]; then
            usage
            exit 1
        fi
        task_state_set_status "$task_id" "$status" "$(date +%F)" "$TASK_STATE_FILE"
        echo "$task_id=$(task_state_get_status "$task_id" "$TASK_STATE_FILE")"
        ;;
    count-remaining)
        range="${1:-}"
        if [[ -z "$range" ]]; then
            usage
            exit 1
        fi
        task_state_count_remaining "$range" "$TASK_STATE_FILE"
        ;;
    summary)
        task_state_summary_counts "$TASK_STATE_FILE"
        ;;
    -h|--help|help)
        usage
        ;;
    *)
        echo "Unknown command: $cmd" >&2
        usage
        exit 1
        ;;
esac
