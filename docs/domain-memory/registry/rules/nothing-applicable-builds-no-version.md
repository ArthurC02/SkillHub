---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L434-438"
    excerpt_sha256: "sha256:e3edd4f60510e178b0fadaeeb3d21b925c279aa5d2ebb6acaf898e79ef43ff10"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_integration_test.go#L565-626"
    excerpt_sha256: "sha256:f6b83c3c2e5ccdddf06bfdad1f67e9c4d9eeb2dad40b5887c395522d562d532d"
---
When every selected suggestion is blocked, the apply returns the rejections and builds no version.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- All suggestions blocked answers 422 and the version count is unchanged.
