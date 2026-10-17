---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/tool_actions.go#L135-148"
    excerpt_sha256: "sha256:d9bb367a7ac20e28ef77c5351f7dbf0b19056789355d293b0354294c204b516d"
---
Each new fetch the model asks for waits for the person to confirm that exact URL; an earlier approval never carries over.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- After approving one URL, a new fetch request is pending with no approved URL.
