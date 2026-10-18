---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T16:04:44.915924Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/testcase.go#L132-141"
    excerpt_sha256: "sha256:154d5695c8dade24873636faffaea65c378986ef7b27255da95fb90f0279baee"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/packaging_integration_test.go#L938-981"
    excerpt_sha256: "sha256:0a33536ee092756a26aad508c2657e692e1cf71b756fcde1d444476415a1b57d"
---
A test case from an uncurated source that carries a dataset stays out of the package as user_uploaded_dataset; one without a dataset stays out as not_curated.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A user's own case with private.csv is excluded as user_uploaded_dataset.
