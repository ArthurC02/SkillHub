---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:45:42.836437Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L222-222"
    excerpt_sha256: "sha256:35fa68a17c790b5d0611f76f22885b12bf1fb3c789707cd36fe5b4ec7bb2cdea"
---
# Relations
- Producer context: [ingest](/contexts/ingest)
- Consumer context: [identity](/contexts/identity)
