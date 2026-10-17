---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/operations/runs.go#L95-100"
    excerpt_sha256: "sha256:46f4a50e5c091e68a826b733aa4d76a8eb398dd1eaf8b2c727f5e9ce7bca1881"
---
A platform agent run with unpriced steps counts toward the daily cap at least the budget its gateway key was given.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A run priced at 0.2 USD with one unpriced step and a 1 USD key budget counts as 1 USD.
