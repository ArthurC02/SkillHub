---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/service.go#L197-202"
    excerpt_sha256: "sha256:20d027d1998fca7a612b64b08e971530d36111821ddcee6dd1903a5499d755f9"
---
An event whose run_id or attempt differs from the ingestion token's grant is refused and counted as rejected.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- An event naming another Run's id under this Run's token is not stored.
