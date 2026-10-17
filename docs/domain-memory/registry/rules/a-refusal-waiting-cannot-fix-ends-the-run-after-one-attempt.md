---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T18:18:26.030858Z"
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L324-343"
    excerpt_sha256: "sha256:21a04e1f74e5b9d64dc51b2fb0a81208c3d70ca3347ac94b95061b1664dab3f5"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/run_dispatch_characterization_test.go#L211-227"
    excerpt_sha256: "sha256:e2a28869548a08225d5a0aa3c2311cc050c9a703ca2b830cb3b48e512d060918"
---
A provider refusal that is neither unavailable nor full is final, so no further attempt starts.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A provider answering 400 leaves the run failed after exactly one attempt.
