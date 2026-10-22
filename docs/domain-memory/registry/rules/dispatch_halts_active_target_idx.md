---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0030_dispatch_halts.sql#L15-16"
    excerpt_sha256: "sha256:f918b95863f304d5ea79eb8729e8e912fc67a2f58760e42fb4076710db2d6267"
---
dispatch_halts keeps unique-index dispatch_halts_active_target_idx: CREATE UNIQUE INDEX dispatch_halts_active_target_idx ON dispatch_halts (provider) WHERE lifted_at IS NULL

# Relations
- Contexts: [run](/contexts/run)
