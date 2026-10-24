---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/http.go#L21-21"
    excerpt_sha256: "sha256:8cee0c16c5e75f70fdd226250a83230fccd5ffbe59745d5047859aabcbed27ff"
  - resource: "apps/platform/internal/trial/evidence/http.go#L60-63"
    excerpt_sha256: "sha256:81de9a87479fee0bec3492b389a487dd4cedeb005e38bdf25df8c572e0265db7"
  - resource: "apps/platform/internal/trial/evidence/http_test.go#L48-59"
    excerpt_sha256: "sha256:f172c0e4a2593829b92fc13dc0d4697666a7379caf20de448fe031ee973d44ba"
---
One trace ingestion request carries at most 1000 events; a larger batch is refused with 413 before any event is judged.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- 1000 events reach ingestion; 1001 events answer 413 too many events in one batch.
