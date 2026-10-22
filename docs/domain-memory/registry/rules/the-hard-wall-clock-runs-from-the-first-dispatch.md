---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T10:07:44.566259Z"
sources:
  - resource: "apps/platform/internal/trial/execution/schedule_test.go#L376-399"
    excerpt_sha256: "sha256:b1620d0d6727c1258ab54edd3923c640130725457f56f3b5d9e9152dd591b625"
  - resource: "apps/platform/internal/trial/execution/job.go#L917-967"
    excerpt_sha256: "sha256:e2735d2dcbee29c8010dbf873af8badd3949d85a9094ee7a4c4f32775473c8e0"
---
Once a provider has accepted an attempt, the run's deadline is its first dispatch plus the frozen policy's hard wall clock, not its creation time.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- Dispatches at 13:00 and 14:00 under a 120-second policy give a 13:02 deadline.
