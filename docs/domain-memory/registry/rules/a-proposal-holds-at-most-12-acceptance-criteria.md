---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-26T03:31:47.051282Z"
sources:
  - resource: "apps/platform/internal/creator/creation/proposal.go#L34-45"
    excerpt_sha256: "sha256:4e592b8c7e6817e2b5d170d89630e63aeccaa33fc7891c16d2c9d624c69bee2b"
  - resource: "apps/platform/internal/creator/creation/proposal_test.go#L119-134"
    excerpt_sha256: "sha256:097778f802025133658739675f12d98c2daff370bb256700f673e1634ed03270"
---
A model reply with more than 12 acceptance criteria is rejected.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- 13 criteria are rejected.
