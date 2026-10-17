---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T04:33:27.317410Z"
sources:
  - resource: "apps/platform/internal/product/entitlements/generate_quota.go#L28-34"
    excerpt_sha256: "sha256:76d93e91db9308763a759ad239897ad8e7d6f0b52101dd71046777d94717ec4e"
  - resource: "apps/platform/internal/product/entitlements/generate_quota_test.go#L22-49"
    excerpt_sha256: "sha256:efca75e19a8923db91558b42c740acbf59912a89bd202fb0242d9f8e5fd4d264"
---
Generations and runs have separate allowances: a spent generation allowance refuses with ErrGenerateQuotaExceeded, never the run refusal, and the reverse.

# Relations
- Contexts: [policy](/contexts/policy)

# Examples
- A Workspace out of generations gets ErrGenerateQuotaExceeded, not ErrQuotaExceeded.
