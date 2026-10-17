---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L921-958"
    excerpt_sha256: "sha256:03f9ac092c8fd18dae138c4065676dad149bbb0c0e34ab6a88b6833dfe033b8a"
---
Once a provider has accepted an attempt, the run's deadline is its first dispatch plus the frozen policy's hard wall clock, not its creation time.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- Dispatches at 13:00 and 14:00 under a 120-second policy give a 13:02 deadline.
