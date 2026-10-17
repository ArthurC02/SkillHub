---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T18:18:26.030858Z"
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L921-958"
    excerpt_sha256: "sha256:03f9ac092c8fd18dae138c4065676dad149bbb0c0e34ab6a88b6833dfe033b8a"
  - resource: "apps/platform/internal/trial/execution/schedule_test.go#L376-399"
    excerpt_sha256: "sha256:b1620d0d6727c1258ab54edd3923c640130725457f56f3b5d9e9152dd591b625"
---
Once a provider has accepted an attempt, the run's deadline is its first dispatch plus the frozen policy's hard wall clock, not its creation time.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- Dispatches at 13:00 and 14:00 under a 120-second policy give a 13:02 deadline.
