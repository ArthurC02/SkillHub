---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0117_run_continuations.sql#L28-28"
    excerpt_sha256: "sha256:ff13942b3f4df22bf394a8c75764851d84d2350eb25745ab691812013a0566c2"
---
run_settings keeps check run_settings.reason: reason text NOT NULL CHECK (btrim(reason) <> '')

# Relations
- Contexts: [run](/contexts/run)
