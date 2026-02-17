#!/usr/bin/env bash
#
# phases-lib.sh -- Dynamic phase definitions parsed from phases.conf
#
# Parses docs/tasks/phases.conf and exposes:
#
#   Arrays/strings:
#     ALL_PHASE_IDS   - Indexed array of phase IDs (for phase-pipeline.sh)
#     ALL_PHASES       - Space-separated string of phase IDs (for ralph-lib.sh)
#     TOTAL_PHASES     - Number of phases
#     FIRST_PHASE_ID   - First phase ID
#     LAST_PHASE_ID    - Last phase ID
#     TOTAL_TASK_COUNT - Sum of all task ranges
#
#   Ralph-lib compat variables (set per phase):
#     PHASE_RANGES_N   - "T-XXX:T-YYY" (accessed via ${!var} indirection)
#     PHASE_NAMES_N    - "Phase N: DisplayName"
#
#   Functions:
#     phase_slug <id>           - Kebab-case slug for branch names
#     phase_title <id>          - Human-readable display name
#     _phase_icon <id>          - Emoji icon
#     validate_single_phase <id> - Returns 0 if valid phase ID
#     get_phase_range <id>      - "T-XXX:T-YYY"
#     get_phase_name <id>       - "Phase N: DisplayName"
#     get_phase_for_task <T-XXX> - Phase ID containing this task
#     phases_listing             - Formatted listing for usage() text
#
# Expects PROJECT_ROOT to be set before sourcing.
# Falls back to deriving it from this script's directory.
#

# Guard against double-sourcing
if [[ -n "${_PHASES_LIB_LOADED:-}" ]]; then
    return 0
fi
_PHASES_LIB_LOADED=1

# Resolve phases.conf path
: "${PROJECT_ROOT:=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
_PHASES_CONF="${PROJECT_ROOT}/docs/tasks/phases.conf"

if [[ ! -f "$_PHASES_CONF" ]]; then
    echo "FATAL: phases.conf not found at $_PHASES_CONF" >&2
    exit 1
fi

# =============================================================================
# Parse phases.conf
# =============================================================================

ALL_PHASE_IDS=()
ALL_PHASES=""
TOTAL_TASK_COUNT=0

while IFS='|' read -r _pid _pslug _ptitle _pstart _pend _picon || [[ -n "$_pid" ]]; do
    # Skip comments and blank lines
    _pid="${_pid## }"; _pid="${_pid%% }"
    [[ -z "$_pid" || "$_pid" == \#* ]] && continue

    ALL_PHASE_IDS+=("$_pid")
    ALL_PHASES="${ALL_PHASES:+$ALL_PHASES }$_pid"

    # Store per-phase data via dynamic variable names (bash 3.2+ compatible)
    printf -v "_PHASE_SLUG_${_pid}"       '%s' "$_pslug"
    printf -v "_PHASE_TITLE_${_pid}"      '%s' "$_ptitle"
    printf -v "_PHASE_ICON_${_pid}"       '%s' "$_picon"
    printf -v "_PHASE_TASK_START_${_pid}" '%s' "$_pstart"
    printf -v "_PHASE_TASK_END_${_pid}"   '%s' "$_pend"

    # Ralph-lib compat: PHASE_RANGES_N and PHASE_NAMES_N
    printf -v "PHASE_RANGES_${_pid}" '%s' "T-${_pstart}:T-${_pend}"
    printf -v "PHASE_NAMES_${_pid}"  '%s' "Phase ${_pid}: ${_ptitle}"

    # Running task total
    _ts=$((10#$_pstart)); _te=$((10#$_pend))
    TOTAL_TASK_COUNT=$((TOTAL_TASK_COUNT + _te - _ts + 1))
done < "$_PHASES_CONF"

# Clean up parser temporaries
unset _pid _pslug _ptitle _pstart _pend _picon _ts _te

TOTAL_PHASES=${#ALL_PHASE_IDS[@]}
FIRST_PHASE_ID="${ALL_PHASE_IDS[0]}"
LAST_PHASE_ID="${ALL_PHASE_IDS[$((TOTAL_PHASES - 1))]}"

if [[ "$TOTAL_PHASES" -eq 0 ]]; then
    echo "FATAL: No phases parsed from $_PHASES_CONF" >&2
    exit 1
fi

# =============================================================================
# Lookup Functions
# =============================================================================

phase_slug() {
    local _var="_PHASE_SLUG_$1"
    echo "${!_var:-}"
}

phase_title() {
    local _var="_PHASE_TITLE_$1"
    echo "${!_var:-Unknown}"
}

_phase_icon() {
    local _var="_PHASE_ICON_$1"
    echo "${!_var:-📦}"
}

validate_single_phase() {
    local _var="_PHASE_SLUG_$1"
    [[ -n "${!_var:-}" ]]
}

get_phase_range() {
    local _var="PHASE_RANGES_$1"
    echo "${!_var:-}"
}

get_phase_name() {
    local _var="PHASE_NAMES_$1"
    echo "${!_var:-}"
}

get_phase_for_task() {
    local task_id="$1"
    local task_num=$((10#${task_id#T-}))

    local _id
    for _id in "${ALL_PHASE_IDS[@]}"; do
        local _sv="_PHASE_TASK_START_${_id}"
        local _ev="_PHASE_TASK_END_${_id}"
        local _s=$((10#${!_sv}))
        local _e=$((10#${!_ev}))
        if [[ $task_num -ge $_s && $task_num -le $_e ]]; then
            echo "$_id"
            return 0
        fi
    done
    echo ""
}

# Print formatted phase listing for usage() text
phases_listing() {
    local _id
    for _id in "${ALL_PHASE_IDS[@]}"; do
        local _tv="_PHASE_TITLE_${_id}"
        local _sv="_PHASE_TASK_START_${_id}"
        local _ev="_PHASE_TASK_END_${_id}"
        printf '  %-4s %s (T-%s to T-%s)\n' \
            "$_id" "${!_tv}" "${!_sv}" "${!_ev}"
    done
    printf '  %-4s %s\n' "all" "Run all phases sequentially"
}
