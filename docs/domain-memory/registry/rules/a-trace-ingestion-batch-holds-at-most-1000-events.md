---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/http.go#L20-20"
    excerpt_sha256: "sha256:15c38985632ac58e03ab6c4758c9bc4ecff83ae59196bd5159b89f1c7302c840"
  - resource: "apps/platform/internal/trial/evidence/http.go#L59-62"
    excerpt_sha256: "sha256:81de9a87479fee0bec3492b389a487dd4cedeb005e38bdf25df8c572e0265db7"
---
One trace ingestion request carries at most 1000 events; a larger batch is refused with 413 before any event is judged.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- 1000 events reach ingestion; 1001 events answer 413 too many events in one batch.
