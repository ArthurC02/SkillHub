---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T17:11:22.999935Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/github.go#L133-142"
    excerpt_sha256: "sha256:93ed9e3b5f4393880b3a552f0e327ef9d93d5ba9b081ca0eb4ca8f29023f0b2d"
  - resource: "apps/platform/internal/creator/workspace/github_test.go#L110-125"
    excerpt_sha256: "sha256:185caf4df0c030c04213961420804078139e8f33adcca3599c10c45df6b90d2f"
---
A GitHub login whose profile carries no email is refused when no address is both primary and verified, so an unverified address can never claim an account.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A GitHub account whose only primary address is unverified is refused.
