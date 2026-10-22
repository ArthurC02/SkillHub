---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0030_dispatch_halts.sql#L12-12"
    excerpt_sha256: "sha256:840555b0ee7d1e7dd133fa047045312d009e1cfd72e010333cbead41df618a42"
---
dispatch_halts keeps check dispatch_halts:12: CHECK ((lifted_at IS NULL) = (lift_reason IS NULL))

# Relations
- Contexts: [run](/contexts/run)
