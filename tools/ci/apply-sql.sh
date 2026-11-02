#!/usr/bin/env bash
# Usage: apply-sql.sh <file.sql>... -- <psql command that reads SQL on stdin>
# Applies the files in order through one psql session, each in its own transaction.
set -euo pipefail

files=()
while [ "$#" -gt 0 ] && [ "$1" != "--" ]; do
	files+=("$1")
	shift
done
if [ "$#" -lt 2 ] || [ "${#files[@]}" -eq 0 ]; then
	echo "usage: apply-sql.sh <file.sql>... -- <psql command...>" >&2
	exit 2
fi
shift

log="$(mktemp)"
trap 'rm -f "$log"' EXIT

for index in "${!files[@]}"; do
	printf "SELECT 'apply-sql: %d';\nBEGIN;\n" "$index"
	cat "${files[$index]}"
	printf '\nCOMMIT;\n'
done | "$@" -q -v ON_ERROR_STOP=1 >"$log" && exit 0

index="$(sed -n 's/^ *apply-sql: \([0-9]*\) *$/\1/p' "$log" | tail -n 1)"
failed=unknown
if [ -n "$index" ]; then
	failed="${files[$index]}"
fi
echo "::error file=$failed::$(basename "$failed") failed to apply" >&2
exit 1
