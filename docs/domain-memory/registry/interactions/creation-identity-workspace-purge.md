---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-14T16:38:05.093087Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L217-217"
    excerpt_sha256: "sha256:ea89b07811a7efa9e43358783976b8abcd445f7abeec7a8b1153e022588820fd"
---
# Relations
- Producer context: [creation](/contexts/creation)
- Consumer context: [identity](/contexts/identity)
