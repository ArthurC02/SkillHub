---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/service.go#L193-195"
    excerpt_sha256: "sha256:0dcf0936d7823f7a0d6662fcf7e9442cd034a8d2902a990df6d9afc71886c152"
---
An event sent under a sandbox ingestion token is refused unless the sandbox emitted it; orchestrator and llm_service events cannot arrive that way.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- An orchestrator event sent with the sandbox token is counted as rejected and not stored.
