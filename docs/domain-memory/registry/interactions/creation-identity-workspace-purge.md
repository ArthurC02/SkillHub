---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:45:42.836437Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L218-218"
    excerpt_sha256: "sha256:ea89b07811a7efa9e43358783976b8abcd445f7abeec7a8b1153e022588820fd"
---
# Relations
- Producer context: [creation](/contexts/creation)
- Consumer context: [identity](/contexts/identity)
