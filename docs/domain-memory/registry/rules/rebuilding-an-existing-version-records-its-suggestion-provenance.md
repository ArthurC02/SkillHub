---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-06T19:38:45.654816Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_integration_test.go#L470-526"
    excerpt_sha256: "sha256:54d0b65ca0cf0d1d222e01b4d2110a758fbc6439e45e37d80a9963a5a945d5f8"
  - resource: "apps/platform/internal/trial/improvement/apply.go#L608-612"
    excerpt_sha256: "sha256:cfcf526f14c448cc9afce75936d911d381582f4295238254cff11951ea713c77"
---
When applying suggestions rebuilds an existing version, the suggestions are recorded against that version.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Restoring an earlier text records the suggestion as applied to the earlier version.
