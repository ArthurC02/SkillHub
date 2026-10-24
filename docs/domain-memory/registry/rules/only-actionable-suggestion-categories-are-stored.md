---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/suggest.go#L189-199"
    excerpt_sha256: "sha256:43813eb17f4af906a42facf88e98ba70fd1a825d2f4ae145d2a0d834812682be"
  - resource: "apps/platform/internal/trial/improvement/suggest_test.go#L98-123"
    excerpt_sha256: "sha256:1dc7f540f9c7ac1ad6aed482f041336655f67d5af04952e259fe10f8c584e17e"
---
Only skill, runtime, tool and dataset suggestions are stored; MCP and unknown categories are dropped.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A proposal in category mcp is not stored.
