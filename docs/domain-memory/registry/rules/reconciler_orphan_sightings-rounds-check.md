---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0021_reconciler_orphan_sightings.sql#L6-6"
    excerpt_sha256: "sha256:a949f76a200974d9d3851181e6b680ae22d5950c4c2fdc52295f7df029e9a88c"
---
reconciler_orphan_sightings keeps check reconciler_orphan_sightings.rounds: rounds integer NOT NULL DEFAULT 1 CHECK (rounds > 0)

# Relations
- Contexts: [run](/contexts/run)
