---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T10:07:44.566259Z"
sources:
  - resource: "apps/platform/internal/trial/execution/schedule_test.go#L401-419"
    excerpt_sha256: "sha256:b1b0177c40cc82c1deee4e43c7def823453d2e5efa3b4aeb19e5a60f5935612e"
  - resource: "apps/platform/internal/trial/execution/job.go#L941-972"
    excerpt_sha256: "sha256:322812d354dc8845fbfdaebad4c2ebc8efb34dc941eb004fb3c289430843215f"
---
A run no provider has accepted waits for a slot up to the slot-wait limit plus the time dispatch was paused for it, and is timed out with a queue reason just past that.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A run with no provider handle is still waiting at creation plus the limit, and expired a moment later.
