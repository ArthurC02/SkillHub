---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-14T16:38:05.093087Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L214-214"
    excerpt_sha256: "sha256:1c1fd9c80da744da54ca7a2842316b3c7160e20eb9c631fa6ae9d7adcc0be275"
---
# Relations
- Producer context: [testlab](/contexts/testlab)
- Consumer context: [identity](/contexts/identity)
