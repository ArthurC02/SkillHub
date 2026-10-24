---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/product/activity/service.go#L358-359"
    excerpt_sha256: "sha256:52265aaa7d3123f0fe9cccf6863bbe4423b4cbfafe962b538dcda46091adf3e6"
  - resource: "apps/platform/internal/product/activity/limits_test.go#L29-50"
    excerpt_sha256: "sha256:4c97c494bc46ee998a229118d0f306a498c34355740084ff34d3699c4bd0d62e"
---
A cursor naming a priority rank outside 0 to 3 is refused as invalid, so a forged cursor cannot reach a rank the feed does not have.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- A cursor with rank 4 is refused; rank 3 is accepted.
