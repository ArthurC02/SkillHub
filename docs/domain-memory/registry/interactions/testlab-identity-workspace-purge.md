---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T22:48:02.453867Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L215-215"
    excerpt_sha256: "sha256:1c1fd9c80da744da54ca7a2842316b3c7160e20eb9c631fa6ae9d7adcc0be275"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L50-50"
    excerpt_sha256: "sha256:9f291ec09e82f7aac8c977e0ff646df2a1963d548b316d378ea85bc222707cce"
---
# Relations
- Producer context: [testlab](/contexts/testlab)
- Consumer context: [identity](/contexts/identity)
