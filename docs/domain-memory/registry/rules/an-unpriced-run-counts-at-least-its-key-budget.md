---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:14:38.912433Z"
sources:
  - resource: "apps/platform/internal/product/operations/runs.go#L95-100"
    excerpt_sha256: "sha256:46f4a50e5c091e68a826b733aa4d76a8eb398dd1eaf8b2c727f5e9ce7bca1881"
  - resource: "apps/platform/internal/product/operations/spend_test.go#L9-25"
    excerpt_sha256: "sha256:31683f5e6f146f2cabea222e6120d3705d0dbe00bb0dc18cab868622df138bae"
---
A platform agent run with unpriced steps counts toward the daily cap at least the budget its gateway key was given.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A run priced at 0.2 USD with one unpriced step and a 1 USD key budget counts as 1 USD.
