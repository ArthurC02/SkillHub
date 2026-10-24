---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/http.go#L29-51"
    excerpt_sha256: "sha256:fa4944a4acb327dd5b375b7968ccf41a811cadc8581a9abcc95e7c108cbd95cd"
  - resource: "apps/platform/internal/trial/evidence/http_test.go#L28-46"
    excerpt_sha256: "sha256:87afbc3316bdb801580510e85014275b73b23fd25ce533c0a3ee6ca9c838c4b2"
---
A trace ingestion body over 4 MiB answers 413 as too large, apart from a body that fails while being read, which answers 400.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- A body of 4 MiB and one byte answers 413.
