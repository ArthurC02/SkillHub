---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T02:57:33.844230Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/trust.go#L83-88"
    excerpt_sha256: "sha256:af7fa8141b56077d4c471a57e226a59eb483392b8bdd7dbdd67d65a55cea9748"
  - resource: "apps/platform/internal/skill/discovery/trust_test.go#L67-75"
    excerpt_sha256: "sha256:0370a42ab18f079d8570d97d3d4013a56c193eb5429905fc4b9e44d4a1899b26"
---
A redistribution value the Catalog does not recognise is shown as unknown and not confirmed, never as allowed or blank.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- A value added next year displays exactly as the unknown value does.
