---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L456-463"
    excerpt_sha256: "sha256:f40bf094cb7ca02a90a92de8e08ee6b09264b869e2b46627be8b6915b70a3d09"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_integration_test.go#L716-742"
    excerpt_sha256: "sha256:ea15402b78021d61301a03809502e700423868f826147fdf406d29108d07e332"
---
Suggestions apply only under the skill their evaluation ran; another skill's id is not found.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Applying under an unrelated skill answers 404 and creates no version.
