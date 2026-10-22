---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0030_dispatch_halts.sql#L8-8"
    excerpt_sha256: "sha256:d9db2d79448b27e8726c99408a8ad75ccbb2f6c58a8b916f489daacc78ac5226"
---
dispatch_halts keeps check dispatch_halts.clear_rounds: clear_rounds integer NOT NULL DEFAULT 0 CHECK (clear_rounds >= 0)

# Relations
- Contexts: [run](/contexts/run)
