---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/http.go#L28-50"
    excerpt_sha256: "sha256:fa4944a4acb327dd5b375b7968ccf41a811cadc8581a9abcc95e7c108cbd95cd"
---
A trace ingestion body over 4 MiB answers 413 as too large, apart from a body that fails while being read, which answers 400.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- A body of 4 MiB and one byte answers 413.
