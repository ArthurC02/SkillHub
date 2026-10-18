---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:04:37.509165Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:53.057386Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/general.go#L73-82"
    excerpt_sha256: "sha256:b8f24e42cee6199fa889b5cd9b9e13c78c485881f3aca53b667d20ddf98b0cd9"
  - resource: "apps/platform/internal/trial/evidence/general_test.go#L42-82"
    excerpt_sha256: "sha256:161546fc75cbb450aa71af94243a0f51285616d2525cfcc90d7de079ef040e61"
---
A tool call counts as a success only when its outcome is succeeded.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- A tool call that timed out is counted but not as a success.
