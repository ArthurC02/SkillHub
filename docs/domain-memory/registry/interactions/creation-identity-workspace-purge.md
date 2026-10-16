---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T22:48:02.453867Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L218-218"
    excerpt_sha256: "sha256:ea89b07811a7efa9e43358783976b8abcd445f7abeec7a8b1153e022588820fd"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L53-53"
    excerpt_sha256: "sha256:c9cae7c12e9af3a872da5ff34ca3bb70bf99b37ee7b5c6e837e449be66f47be2"
---
# Relations
- Producer context: [creation](/contexts/creation)
- Consumer context: [identity](/contexts/identity)
