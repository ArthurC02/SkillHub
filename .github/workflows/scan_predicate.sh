#!/usr/bin/env sh
# Turn a grype JSON report into the in-toto vulns predicate that gets attested
# to the published image digest (SBX-011, SEC-002 I-04).
#
# The reason this exists at all is one field: `scanned_at`. ADR-022 gave I-04 a
# 30-day validity and said gate A must be able to judge it, but until now the
# only record of when a scan ran was a date a human typed into
# infra/images/README.md — not a thing a probe can read, and not a thing that is
# wrong loudly when it is wrong.
#
# `fixable_critical_high` is carried alongside the raw findings on purpose: it is
# the exact quantity the I-06 gate blocks on, and an admission probe should be
# able to assert it is 0 without re-implementing grype's severity and fix-state
# logic against the finding list.
#
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
