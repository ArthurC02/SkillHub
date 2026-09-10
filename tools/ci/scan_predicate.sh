#!/usr/bin/env sh
# Turns a grype JSON report into the in-toto vulns predicate attested to the
# image digest. `fixable_critical_high` is precomputed so a probe can assert
# it is 0 without re-deriving severity/fix-state.

# Usage: scan_predicate.sh <grype-json> <output-json>
set -eu

in=$1
out=$2
now=$(date -u +%Y-%m-%dT%H:%M:%SZ)

jq --arg now "$now" '{
  scanner: {
    uri: "https://github.com/anchore/grype",
    version: (.descriptor.version // "unknown"),
    db: {
      last_update: (.descriptor.db.status.built // .descriptor.db.built // null)
    },
    result: [
      .matches[] | {
        id: .vulnerability.id,
        severity: .vulnerability.severity,
        package: .artifact.name,
        version: .artifact.version,
        fix_state: .vulnerability.fix.state,
        fixed_in: (.vulnerability.fix.versions // [])
      }
    ]
  },
  metadata: {
    scan_started_on: $now,
    scan_finished_on: $now
  },
  summary: {
    total: (.matches | length),
    by_severity: (
      reduce .matches[] as $m ({}; .[$m.vulnerability.severity] += 1)
    ),
    fixable_critical_high: (
      [.matches[]
       | select(.vulnerability.fix.state == "fixed")
       | select(.vulnerability.severity == "Critical" or .vulnerability.severity == "High")
      ] | length
    )
  }
}' "$in" > "$out"

echo "scanned_at=$now  fixable Critical/High=$(jq -r '.summary.fixable_critical_high' "$out")"
