---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-06T19:38:45.654816Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_integration_test.go#L716-742"
    excerpt_sha256: "sha256:ea15402b78021d61301a03809502e700423868f826147fdf406d29108d07e332"
  - resource: "apps/platform/internal/trial/improvement/apply.go#L480-487"
    excerpt_sha256: "sha256:4bb0367d651ba017074d830fce264c5a98aca1617b78c7dad3b6e6e93ea06650"
---
Suggestions apply only under the skill their evaluation ran; another skill's id is not found.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Applying under an unrelated skill answers 404 and creates no version.
