---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/creation/tool_actions.go#L18-20"
    excerpt_sha256: "sha256:f3327dd177f1f80f221fe351d9f111393a803db19f0fa391e4ff67c62bb6cc3d"
  - resource: "apps/platform/internal/creator/creation/proposal_test.go#L641-650"
    excerpt_sha256: "sha256:18d24ece48fa1c365edf0aa57c73dc1d28131af22f70531bbc2799d38c662ce1"
---
At the tool-call ceiling a creation's tool request is refused and the count does not grow.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- A tool request at the ceiling is refused as over the limit.
