---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L584-588"
    excerpt_sha256: "sha256:11f3853cc4c76fd9536b1d730ca1fba76e0bca42a1c176af31d38b489d26fa7c"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_integration_test.go#L470-526"
    excerpt_sha256: "sha256:54d0b65ca0cf0d1d222e01b4d2110a758fbc6439e45e37d80a9963a5a945d5f8"
---
When applying suggestions rebuilds an existing version, the suggestions are recorded against that version.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Restoring an earlier text records the suggestion as applied to the earlier version.
