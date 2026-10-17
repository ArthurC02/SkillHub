---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T17:11:22.999935Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/http.go#L120-124"
    excerpt_sha256: "sha256:6a570ea78e1c8c2a9058de2411a3e61f5b45c3ebb397084a1d0feda7a90b7ced"
  - resource: "apps/platform/internal/creator/workspace/github_test.go#L183-209"
    excerpt_sha256: "sha256:c30f42ad3b3446dbbf8bfedb0b1f66682d4f0719e3769bb1963ef655d79791d8"
---
A GitHub login callback is refused with 401 before any code exchange unless its state matches the state cookie set when the login began.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A callback with state abc and a state cookie of other answers 401.
