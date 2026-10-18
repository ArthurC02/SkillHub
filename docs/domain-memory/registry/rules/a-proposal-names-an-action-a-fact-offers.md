---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:59:29.446208Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:54.156118Z"
sources:
  - resource: "apps/platform/internal/product/operations/daily_report.go#L66-68"
    excerpt_sha256: "sha256:f37e66f6e9cf906c3beb08e30edd83bfbf5423a8ce47f5c0703fb55ff766f24c"
  - resource: "apps/platform/internal/product/operations/daily_report_test.go#L46-71"
    excerpt_sha256: "sha256:713fc408716a8673f260fbe0525b331263ae54026d9d65b4b0d0723bb5e61595"
---
A report's proposal is rejected unless a fact a tool returned offers the action it names.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- Proposing an action no returned fact offers fails the report.
