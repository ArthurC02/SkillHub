---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-14T16:38:05.093087Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L216-216"
    excerpt_sha256: "sha256:70a173d82b2084fc3ef5a8c12d92b0e906fb2377aee147656437a51492fd3066"
---
# Relations
- Producer context: [packaging](/contexts/packaging)
- Consumer context: [identity](/contexts/identity)
