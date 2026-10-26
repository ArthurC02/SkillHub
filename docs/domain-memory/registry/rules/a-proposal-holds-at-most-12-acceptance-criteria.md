---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-26T03:31:47.051282Z"
sources:
  - resource: "apps/platform/internal/creator/creation/proposal.go#L34-63"
    excerpt_sha256: "sha256:aa1249f62aa4e31a1d0b8c709774e4b42cef3ca4650d311b734125c2cd1978d7"
  - resource: "apps/platform/internal/creator/creation/proposal_test.go#L119-134"
    excerpt_sha256: "sha256:097778f802025133658739675f12d98c2daff370bb256700f673e1634ed03270"
---
A model reply with more than 12 acceptance criteria is rejected.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- 13 criteria are rejected.
