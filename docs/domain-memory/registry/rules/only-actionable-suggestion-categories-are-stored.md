---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/suggest.go#L152-162"
    excerpt_sha256: "sha256:43813eb17f4af906a42facf88e98ba70fd1a825d2f4ae145d2a0d834812682be"
---
Only skill, runtime, tool and dataset suggestions are stored; MCP and unknown categories are dropped.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A proposal in category mcp is not stored.
