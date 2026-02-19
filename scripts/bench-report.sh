#!/usr/bin/env bash
# bench-report.sh -- Run all Raven benchmarks and format the results.
#
# Usage:
#   ./scripts/bench-report.sh [options]
#
# Options:
#   -t BENCHTIME   Per-benchmark run time (default: 3s)
#   -c COUNT       Number of runs per benchmark (default: 1)
#   -p PACKAGE     Restrict to a specific package path (default: ./...)
#   -r REGEXP      Benchmark filter regexp (default: .)
#   -o OUTPUT      Write raw output to this file (default: stdout only)
#   -h             Show this help message

set -euo pipefail

BENCHTIME="${BENCHTIME:-3s}"
COUNT="${COUNT:-1}"
PACKAGE="${PACKAGE:-./...}"
REGEXP="${REGEXP:-.}"
OUTPUT=""

usage() {
    grep '^#' "$0" | sed 's/^# \?//'
    exit 0
}

while getopts "t:c:p:r:o:h" opt; do
    case "$opt" in
        t) BENCHTIME="$OPTARG" ;;
        c) COUNT="$OPTARG"     ;;
        p) PACKAGE="$OPTARG"   ;;
        r) REGEXP="$OPTARG"    ;;
        o) OUTPUT="$OPTARG"    ;;
        h) usage               ;;
        *) usage               ;;
    esac
done

TIMESTAMP="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
GIT_REF="$(git describe --tags --always --dirty 2>/dev/null || echo "unknown")"
GO_VERSION="$(go version)"

header() {
    echo "=========================================="
    echo "  Raven Performance Benchmark Report"
    echo "=========================================="
    echo "  Timestamp : ${TIMESTAMP}"
    echo "  Git ref   : ${GIT_REF}"
    echo "  Go        : ${GO_VERSION}"
    echo "  Package   : ${PACKAGE}"
    echo "  Run time  : -benchtime=${BENCHTIME}"
    echo "  Count     : -count=${COUNT}"
    echo "  Filter    : -bench=${REGEXP}"
    echo "=========================================="
    echo ""
}

run_benchmarks() {
    local args=(
        -bench="${REGEXP}"
        -benchmem
        -benchtime="${BENCHTIME}"
        -count="${COUNT}"
        -run='^$'   # skip unit tests
    )

    echo "Running: go test ${PACKAGE} ${args[*]}"
    echo ""

    go test "${PACKAGE}" "${args[@]}"
}

if [[ -n "$OUTPUT" ]]; then
    {
        header
        run_benchmarks
    } | tee "$OUTPUT"
    echo ""
    echo "Raw results written to: ${OUTPUT}"
else
    header
    run_benchmarks
fi
