---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-26T03:31:47.051282Z"
sources:
  - resource: "apps/platform/internal/creator/creation/tool_actions.go#L133-146"
    excerpt_sha256: "sha256:d9bb367a7ac20e28ef77c5351f7dbf0b19056789355d293b0354294c204b516d"
  - resource: "apps/platform/internal/creator/creation/attempt_test.go#L399-411"
    excerpt_sha256: "sha256:54244f560d249db6b1ece5d95e8f843014ad4d3c4160cb63ea17ed610f5a4e90"
---
Each new fetch the model asks for waits for the person to confirm that exact URL; an earlier approval never carries over.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- After approving one URL, a new fetch request is pending with no approved URL.
