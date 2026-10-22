---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0030_dispatch_halts.sql#L5-5"
    excerpt_sha256: "sha256:a70e67dd418df1edae882b62e5aeca4fdba812d382be99452a2a49d32455153c"
---
dispatch_halts keeps check dispatch_halts.reason: reason text NOT NULL CHECK (btrim(reason) <> '')

# Relations
- Contexts: [run](/contexts/run)
