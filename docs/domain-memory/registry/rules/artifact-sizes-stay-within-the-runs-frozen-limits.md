---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L670-700"
    excerpt_sha256: "sha256:7469ca0c5c2165b05ca13a7629d34c7fa2a64c305523e3a72972ed89af0a981e"
---
Each reported artifact must be no larger than the run's frozen per-file limit, and their total within its frozen total limit, before anything is persisted.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A file one byte over the per-file limit is refused.
