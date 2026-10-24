---
type: Interaction
status: stable
consistency: synchronous
delivery: "composition-root hook inside the caller's transaction"
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/app.go#L226-226"
    excerpt_sha256: "sha256:35fa68a17c790b5d0611f76f22885b12bf1fb3c789707cd36fe5b4ec7bb2cdea"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L73-73"
    excerpt_sha256: "sha256:bdfe758737c27fdd6053232ab772da1a38b8eeed93ff6b4b8d75580f9cab2d05"
---
# Relations
- Producer context: [ingest](/contexts/ingest)
- Consumer context: [identity](/contexts/identity)
