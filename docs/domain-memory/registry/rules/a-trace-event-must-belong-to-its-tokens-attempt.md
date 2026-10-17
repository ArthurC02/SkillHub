---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T12:47:32.093363Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/service.go#L197-202"
    excerpt_sha256: "sha256:20d027d1998fca7a612b64b08e971530d36111821ddcee6dd1903a5499d755f9"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/trace_integration_test.go#L602-638"
    excerpt_sha256: "sha256:728df401b9a0a8df88392eaeec840c728d77c5290f105073227ffc34cd976441"
---
An event whose run_id or attempt differs from the ingestion token's grant is refused and counted as rejected.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- An event naming another Run's id under this Run's token is not stored.
