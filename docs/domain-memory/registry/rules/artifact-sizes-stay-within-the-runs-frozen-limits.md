---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L681-711"
    excerpt_sha256: "sha256:7469ca0c5c2165b05ca13a7629d34c7fa2a64c305523e3a72972ed89af0a981e"
  - resource: "apps/platform/internal/trial/execution/provider_response_test.go#L31-56"
    excerpt_sha256: "sha256:fdce51c649e64c873b8cfdfffbb43405220acaa1d3f2c8af562b153c2b733438"
---
Each reported artifact must be no larger than the run's frozen per-file limit, and their total within its frozen total limit, before anything is persisted.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A file one byte over the per-file limit is refused.
