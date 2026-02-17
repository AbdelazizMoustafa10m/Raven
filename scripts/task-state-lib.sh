#!/usr/bin/env bash
#
# task-state-lib.sh -- Machine-readable task-state helpers
#
# Canonical state file format (pipe-delimited):
#   T-001|completed|2026-02-16
#   T-016|not_started|2026-02-16
#
# Status values:
#   completed | not_started | in_progress | blocked
#

# Guard against double-sourcing
if [[ -n "${_TASK_STATE_LIB_LOADED:-}" ]]; then
    return 0
fi
_TASK_STATE_LIB_LOADED=1

# Resolve project root if not provided
: "${PROJECT_ROOT:=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
: "${TASK_STATE_FILE:=${PROJECT_ROOT}/docs/tasks/task-state.conf}"

# Load phase metadata if not already loaded (needed for init/count helpers)
if [[ -z "${_PHASES_LIB_LOADED:-}" ]]; then
    source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/phases-lib.sh"
fi

task_state_normalize_status() {
    local raw="${1:-}"
    raw=$(echo "$raw" | tr '[:upper:]' '[:lower:]' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
    case "$raw" in
        completed|complete|done)
            echo "completed"
            ;;
        not_started|"not started"|todo|pending)
            echo "not_started"
            ;;
        in_progress|"in progress"|active)
            echo "in_progress"
            ;;
        blocked)
            echo "blocked"
            ;;
        *)
            echo ""
            ;;
    esac
}

task_state_validate_task_id() {
    local task_id="$1"
    [[ "$task_id" =~ ^T-[0-9]{3}$ ]]
}

task_state_get_status() {
    local task_id="$1"
    local state_file="${2:-$TASK_STATE_FILE}"
    task_state_validate_task_id "$task_id" || return 1
    [[ -f "$state_file" ]] || return 1

    local status
    status=$(awk -F'|' -v task="$task_id" '
        $0 !~ /^[[:space:]]*#/ && NF >= 2 {
            id=$1
            st=$2
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", id)
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", st)
            if (id == task) {
                print st
                exit
            }
        }
    ' "$state_file")

    task_state_normalize_status "$status"
}

task_state_is_completed() {
    local task_id="$1"
    local state_file="${2:-$TASK_STATE_FILE}"
    local status
    status=$(task_state_get_status "$task_id" "$state_file" 2>/dev/null || true)
    [[ "$status" == "completed" ]]
}

task_state_set_status() {
    local task_id="$1"
    local raw_status="$2"
    local updated_at="${3:-$(date +%F)}"
    local state_file="${4:-$TASK_STATE_FILE}"

    task_state_validate_task_id "$task_id" || return 1

    local status
    status=$(task_state_normalize_status "$raw_status")
    [[ -n "$status" ]] || return 1

    mkdir -p "$(dirname "$state_file")"
    if [[ ! -f "$state_file" ]]; then
        task_state_init_file "$state_file" "$updated_at"
    fi

    local tmp_file
    tmp_file=$(mktemp "${state_file}.tmp.XXXXXX")

    awk -F'|' -v task="$task_id" -v st="$status" -v dt="$updated_at" '
        BEGIN { updated = 0 }
        {
            if ($0 ~ /^[[:space:]]*#/ || $0 ~ /^[[:space:]]*$/) {
                print $0
                next
            }

            id=$1
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", id)
            if (id == task) {
                print task "|" st "|" dt
                updated = 1
                next
            }
            print $0
        }
        END {
            if (!updated) {
                print task "|" st "|" dt
            }
        }
    ' "$state_file" > "$tmp_file"

    mv "$tmp_file" "$state_file"
}

task_state_count_remaining() {
    local range="$1"
    local state_file="${2:-$TASK_STATE_FILE}"
    local start="${range%%:*}"
    local end="${range##*:}"
    local start_num=$((10#${start#T-}))
    local end_num=$((10#${end#T-}))

    local remaining=0
    local i
    for ((i = start_num; i <= end_num; i++)); do
        local task_id
        task_id=$(printf "T-%03d" "$i")
        if ! task_state_is_completed "$task_id" "$state_file"; then
            remaining=$((remaining + 1))
        fi
    done
    echo "$remaining"
}

task_state_init_file() {
    local state_file="${1:-$TASK_STATE_FILE}"
    local date_str="${2:-$(date +%F)}"
    local tmp_file
    tmp_file=$(mktemp "${state_file}.tmp.XXXXXX")

    {
        echo "# Raven Task State -- machine-readable source of truth"
        echo "# Format: TASK_ID|STATUS|UPDATED_AT (YYYY-MM-DD)"
        echo "# STATUS: completed | not_started | in_progress | blocked"
        echo ""
    } > "$tmp_file"

    local phase_id
    for phase_id in $ALL_PHASES; do
        local range
        range=$(get_phase_range "$phase_id")
        local start="${range%%:*}"
        local end="${range##*:}"
        local start_num=$((10#${start#T-}))
        local end_num=$((10#${end#T-}))

        local i
        for ((i = start_num; i <= end_num; i++)); do
            printf "T-%03d|not_started|%s\n" "$i" "$date_str" >> "$tmp_file"
        done
    done

    mkdir -p "$(dirname "$state_file")"
    mv "$tmp_file" "$state_file"
}

task_state_sync_from_progress() {
    local progress_file="${1:-${PROJECT_ROOT}/docs/tasks/PROGRESS.md}"
    local state_file="${2:-$TASK_STATE_FILE}"
    [[ -f "$progress_file" ]] || return 1

    local updates_file
    updates_file=$(mktemp "${state_file}.updates.XXXXXX")

    awk '
        function trim(v) {
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", v)
            return v
        }
        function norm(v, lc) {
            lc = tolower(trim(v))
            if (lc == "completed" || lc == "complete" || lc == "done") return "completed"
            if (lc == "not started" || lc == "not_started") return "not_started"
            if (lc == "in progress" || lc == "in_progress") return "in_progress"
            if (lc == "blocked") return "blocked"
            return ""
        }

        match($0, /^### Phase [0-9]+: .*\(T-([0-9]{3}) to T-([0-9]{3})\)/, m) {
            pstart = m[1] + 0
            pend = m[2] + 0
            awaiting = 1
            next
        }

        awaiting && match($0, /^- \*\*Status:\*\* (.+)$/, s) {
            st = norm(s[1])
            if (st != "") {
                for (i = pstart; i <= pend; i++) {
                    printf("T-%03d|%s\n", i, st)
                }
            }
            awaiting = 0
            next
        }

        /^\|/ {
            n = split($0, cells, "|")
            task = ""
            status = ""

            for (i = 1; i <= n; i++) {
                cell = trim(cells[i])
                if (cell ~ /^T-[0-9]{3}$/) {
                    task = cell
                }
            }

            if (task != "") {
                for (i = n; i >= 1; i--) {
                    cell = trim(cells[i])
                    if (cell != "") {
                        status = cell
                        break
                    }
                }

                st = norm(status)
                if (st != "") {
                    print task "|" st
                }
            }
        }
    ' "$progress_file" > "$updates_file"

    local today
    today=$(date +%F)
    while IFS='|' read -r task_id status; do
        [[ -z "$task_id" || -z "$status" ]] && continue
        task_state_set_status "$task_id" "$status" "$today" "$state_file"
    done < "$updates_file"

    rm -f "$updates_file"
}

task_state_ensure_file() {
    local progress_file="${1:-${PROJECT_ROOT}/docs/tasks/PROGRESS.md}"
    local state_file="${2:-$TASK_STATE_FILE}"

    if [[ -f "$state_file" ]]; then
        return 0
    fi

    local today
    today=$(date +%F)
    task_state_init_file "$state_file" "$today"

    if [[ -f "$progress_file" ]]; then
        task_state_sync_from_progress "$progress_file" "$state_file" || true
    fi

    return 0
}

task_state_summary_counts() {
    local state_file="${1:-$TASK_STATE_FILE}"
    [[ -f "$state_file" ]] || return 1

    awk -F'|' '
        function norm(v, lc) {
            lc = tolower(v)
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", lc)
            if (lc == "completed" || lc == "complete" || lc == "done") return "completed"
            if (lc == "not started" || lc == "not_started") return "not_started"
            if (lc == "in progress" || lc == "in_progress") return "in_progress"
            if (lc == "blocked") return "blocked"
            return "unknown"
        }
        $0 !~ /^[[:space:]]*#/ && NF >= 2 {
            st = norm($2)
            counts[st]++
            total++
        }
        END {
            printf "completed=%d\n", counts["completed"] + 0
            printf "in_progress=%d\n", counts["in_progress"] + 0
            printf "not_started=%d\n", counts["not_started"] + 0
            printf "blocked=%d\n", counts["blocked"] + 0
            printf "total=%d\n", total + 0
        }
    ' "$state_file"
}
