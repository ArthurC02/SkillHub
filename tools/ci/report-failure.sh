#!/usr/bin/env bash
# Runs a command and, on failure, publishes its output two ways: a step
# summary and an `::error::` annotation (the one the check-runs API returns).
# Usage: bash tools/ci/report-failure.sh "<title>" <command> [args...]
set -uo pipefail

title=$1
shift

log=$(mktemp)
if "$@" 2>&1 | tee "$log"; then
    exit 0
fi

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    {
        printf '### %s failed\n\n```\n' "$title"
        tail -c 60000 "$log"
        printf '\n```\n'
    } >>"$GITHUB_STEP_SUMMARY"
fi

# An annotation is one line: newlines become %0A, and % is escaped first so
# that step doesn't eat the %0A just written.
printf '::error title=%s failed::' "$title"
tail -c 3000 "$log" |
    sed -e 's/%/%25/g' -e 's/\r$//' |
    awk 'BEGIN { ORS = "" } NR > 1 { print "%0A" } { print }'
printf '\n'

exit 1
