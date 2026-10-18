---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L313-314"
    excerpt_sha256: "sha256:52265aaa7d3123f0fe9cccf6863bbe4423b4cbfafe962b538dcda46091adf3e6"
  - resource: "apps/platform/internal/product/activity/limits_test.go#L29-50"
    excerpt_sha256: "sha256:4c97c494bc46ee998a229118d0f306a498c34355740084ff34d3699c4bd0d62e"
---
A cursor without a kind or without a source id is refused as invalid.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- A cursor with an empty source_id is refused.
