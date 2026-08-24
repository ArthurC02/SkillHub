#!/usr/bin/env bash
# Run a test command and, if it fails, publish its output where an outside
# reader can actually get at it.
#
# Why this exists. A job's step log needs admin rights on the repository, so
# `GET /commits/{sha}/check-runs` hands anyone else the word "failure" and
# nothing else. That is fine until a leg fails only on a machine nobody has --
# gVisor, a Linux runner, an empty database -- and then it is the difference
# between a diagnosis and a guess. Two channels, because they fail differently:
#
#   - the step summary, which renders properly and holds a lot, but is NOT
#     returned in a check run's `output` (measured 2026-08-25: `summary` and
#     `text` both come back null while `annotations_count` is 2);
#   - an `::error::` annotation, which IS returned by the annotations endpoint
#     to anyone who can see the repository, and is therefore the one that
#     actually answers the question. It is truncated, so it carries the tail --
#     which is where `go test` puts the failure summary.
#
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

# An annotation is one line, so newlines become %0A and the percent sign has to
# be escaped first -- doing it second would eat the escapes just written. sed
# and awk rather than python3: a minimal image need not have it, and this does
# not need it.
printf '::error title=%s failed::' "$title"
tail -c 3000 "$log" |
    sed -e 's/%/%25/g' -e 's/\r$//' |
    awk 'BEGIN { ORS = "" } NR > 1 { print "%0A" } { print }'
printf '\n'

exit 1
