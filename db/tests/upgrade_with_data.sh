#!/usr/bin/env bash
# Usage: upgrade_with_data.sh <url of an empty database>
# PSQL may name a psql command that reads SQL from stdin (default: psql).
set -euo pipefail

url="$1"
read -ra psql_cmd <<<"${PSQL:-psql}"
here="$(cd "$(dirname "$0")" && pwd)"
seed="$(ls "$here"/upgrade_seed_*.sql)"
baseline="$(basename "$seed" .sql)"
baseline="${baseline#upgrade_seed_}"

apply() {
  "${psql_cmd[@]}" "$url" -v ON_ERROR_STOP=1 --single-transaction -q <"$1" >/dev/null ||
    { echo "::error file=$1::$(basename "$1") failed on a database that already holds data"; exit 1; }
}

upgraded=0
for migration in "$here"/../migrations/*.sql; do
  number="$(basename "$migration")"
  number="${number%%_*}"
  apply "$migration"
  if [[ "$number" > "$baseline" ]]; then
    upgraded=$((upgraded + 1))
  elif [[ "$number" == "$baseline" ]]; then
    apply "$seed"
  fi
done

if ((upgraded == 0)); then
  echo "::error file=$seed::no migration follows the seed's baseline $baseline, so nothing was upgraded with data"
  exit 1
fi
echo "upgrade with data: $upgraded migration(s) after $baseline applied to a seeded database"
