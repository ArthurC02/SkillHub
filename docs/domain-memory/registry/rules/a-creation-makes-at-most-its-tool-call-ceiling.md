---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-26T03:31:47.051282Z"
sources:
  - resource: "apps/platform/internal/creator/creation/tool_actions.go#L18-20"
    excerpt_sha256: "sha256:f3327dd177f1f80f221fe351d9f111393a803db19f0fa391e4ff67c62bb6cc3d"
  - resource: "apps/platform/internal/creator/creation/proposal_test.go#L603-608"
    excerpt_sha256: "sha256:0ceb2783dba1b635693992fae26638dfa9e7fb86cad6de2ef4be1ca6feecee15"
---
At the tool-call ceiling a creation's tool request is refused and the count does not grow.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- A tool request at the ceiling is refused as over the limit.
