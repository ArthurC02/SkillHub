---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/service.go#L89-101"
    excerpt_sha256: "sha256:0d81ebe449ce94330e2e79a881dda492101bf205d5f5f9c97e47279a1ca365f3"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/trace_integration_test.go#L602-638"
    excerpt_sha256: "sha256:728df401b9a0a8df88392eaeec840c728d77c5290f105073227ffc34cd976441"
---
An invalid event is counted as rejected and skipped; the other events in the same batch are still stored.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- A batch with one event for another attempt stores the rest and reports one rejected.
