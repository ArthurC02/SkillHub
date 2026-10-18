---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:10:47.982008Z"
sources:
  - resource: "apps/platform/internal/product/learning/policy.go#L10-13"
    excerpt_sha256: "sha256:d0d1f13e9be1d9e34422a21d004e6a0c50128274d896ac4cfb15095c0326b950"
  - resource: "apps/platform/internal/product/learning/policy_test.go#L158-179"
    excerpt_sha256: "sha256:c6f370fca0c97ff9383783c0eee6a07686ddd2140693a83a30cadf05dc3db3e9"
---
When no feedback retention window is set, the disclosure shows no day count and says reports are kept until one is set, never zero days.

# Relations
- Contexts: [analytics](/contexts/analytics)

# Examples
- With no window, retention_days is null and a note names the setting.
