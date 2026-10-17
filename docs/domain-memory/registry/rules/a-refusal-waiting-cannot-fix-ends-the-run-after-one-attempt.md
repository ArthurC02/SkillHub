---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L324-343"
    excerpt_sha256: "sha256:21a04e1f74e5b9d64dc51b2fb0a81208c3d70ca3347ac94b95061b1664dab3f5"
---
A provider refusal that is neither unavailable nor full is final, so no further attempt starts.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A provider answering 400 leaves the run failed after exactly one attempt.
