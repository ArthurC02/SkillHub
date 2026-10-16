---
type: Business Rule
status: draft
owner: run-execution
version: unknown
effective_from: unknown
effective_until: unknown
sources:
  - resource: "apps/platform/internal/trial/execution/specification.go#L20-20"
    excerpt_sha256: "sha256:41b96ca1fe2bfcab4d81c3b6dc64ee813a085c582b4a546c5842c4c05467fe24"
  - resource: "apps/platform/internal/trial/execution/specification.go#L153-171"
    excerpt_sha256: "sha256:bdad79a9a14ee92166d7a0ad1ad44858e31f6b2e40b470d242204f24b9fe1a16"
---
A Workspace may have at most one Run in progress; a further request is refused with the workspace_concurrency reason.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A second Run requested while the Workspace's first is still queued or running is refused with workspace_concurrency and audited as a refusal.
- Once the first Run reaches a terminal status, the next request is admitted.
