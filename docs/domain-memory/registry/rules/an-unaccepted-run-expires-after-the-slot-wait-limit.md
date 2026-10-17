---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L949-973"
    excerpt_sha256: "sha256:9e49212b592ff44e29aef0aaff310f307a6db7cfff43827f33a1f6c3ac92c289"
---
A run no provider has accepted expires once the slot wait limit has passed since its creation, and not before.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A run with no provider handle is still waiting at creation plus the limit, and expired a moment later.
