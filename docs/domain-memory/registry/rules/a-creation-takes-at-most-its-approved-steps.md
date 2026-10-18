---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/job.go#L42-42"
    excerpt_sha256: "sha256:ebc27436839a8223198d08adb2fc71a87462c09d90145f2af76329159e43bbf8"
  - resource: "apps/platform/internal/creator/creation/step_cap_test.go#L5-22"
    excerpt_sha256: "sha256:1dfc537fbd136ef793d52c16b30d14db9c705ae8483baa973f11842b6feb073a"
---
A creation may take a step only while it has taken fewer than its approved number of steps, even with budget left.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- With budget left, the last approved step is allowed and none after it.
