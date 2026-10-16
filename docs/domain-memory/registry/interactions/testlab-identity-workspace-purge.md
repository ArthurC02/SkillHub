---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:45:42.836437Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L215-215"
    excerpt_sha256: "sha256:1c1fd9c80da744da54ca7a2842316b3c7160e20eb9c631fa6ae9d7adcc0be275"
---
# Relations
- Producer context: [testlab](/contexts/testlab)
- Consumer context: [identity](/contexts/identity)
