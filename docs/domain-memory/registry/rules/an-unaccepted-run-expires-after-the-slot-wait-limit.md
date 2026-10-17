---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T18:18:26.030858Z"
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L949-973"
    excerpt_sha256: "sha256:9e49212b592ff44e29aef0aaff310f307a6db7cfff43827f33a1f6c3ac92c289"
  - resource: "apps/platform/internal/trial/execution/schedule_test.go#L401-419"
    excerpt_sha256: "sha256:b1b0177c40cc82c1deee4e43c7def823453d2e5efa3b4aeb19e5a60f5935612e"
---
A run no provider has accepted expires once the slot wait limit has passed since its creation, and not before.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A run with no provider handle is still waiting at creation plus the limit, and expired a moment later.
