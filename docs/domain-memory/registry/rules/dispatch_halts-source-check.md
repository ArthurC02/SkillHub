---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0030_dispatch_halts.sql#L4-4"
    excerpt_sha256: "sha256:25f651621de4d56e1fe82ef8d4cc7ccdba5a665c28439dbc84f5baf99bb2ac1e"
---
dispatch_halts keeps check dispatch_halts.source: source text NOT NULL CHECK (source IN ('p1_incident', 'orphan_threshold'))

# Relations
- Contexts: [run](/contexts/run)
