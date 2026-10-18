---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:04:37.509165Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:53.057386Z"
sources:
  - resource: "apps/platform/internal/trial/evidence/general.go#L12-14"
    excerpt_sha256: "sha256:ef2bf7ad590106d2e2ac53418d4bfa64571b3225702bfd36a636dce2190e0869"
  - resource: "apps/platform/internal/trial/evidence/general.go#L53-57"
    excerpt_sha256: "sha256:a589b88f373f50cfec8cb415d005e06741bbc1c2ea6fc7c010b62a82c8912098"
  - resource: "apps/platform/internal/trial/evidence/general_test.go#L21-40"
    excerpt_sha256: "sha256:f41a8f5f999f41ca044563d09fdf7fb9445054b742ea0962c5fa172424208ef9"
---
The general trace lists at most the first 100 errors while still counting all of them.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- A run with 101 errors lists 100 and counts 101.
