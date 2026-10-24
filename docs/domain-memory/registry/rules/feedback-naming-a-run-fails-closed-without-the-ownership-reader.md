---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/product/learning/feedback.go#L112-121"
    excerpt_sha256: "sha256:def305b679e2bff093c10827625a3710c38ae12182e54d957bcae08105529271"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/beta_integration_test.go#L711-725"
    excerpt_sha256: "sha256:6c7f00d7dd276fed95a599c1f5daf6908a69637baa0ba5232f7143ce28cbe536"
---
A feedback report that names a run is refused with a server error when the run ownership reader is not wired, and nothing is stored.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- Without the reader, a report naming a valid run id answers 500 and stores no row.
