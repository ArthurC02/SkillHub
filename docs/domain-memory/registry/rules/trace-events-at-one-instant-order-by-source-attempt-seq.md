---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:04:37.509165Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:53.057386Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/service.go#L504-518"
    excerpt_sha256: "sha256:5f1b340cbf7aa30a231a1d11303465978868603156ddaa9c36d22efdb0f1e494"
  - resource: "apps/platform/internal/trial/evidence/trace_test.go#L285-309"
    excerpt_sha256: "sha256:3ece74ed1c69aa33d8c338f18950f82c7c901e49b11b810a41acd7ab180144f6"
---
Events that occurred at the same instant are ordered by producer, then attempt, then seq.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- Two sandbox events of attempt 1 at one instant list seq 1 before seq 2.
