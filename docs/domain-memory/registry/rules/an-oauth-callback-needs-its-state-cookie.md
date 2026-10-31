---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-31T06:34:02.458664Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/http.go#L125-129"
    excerpt_sha256: "sha256:7dda39429e2b4b0e1bde6ed750395a335aee6bfd0df2b6d9764b1dc194585068"
  - resource: "apps/platform/internal/creator/workspace/github_test.go#L184-210"
    excerpt_sha256: "sha256:20ca5eb66b91bf610674bf81c2a434089b3b8563c6937f7bf84e7133dd6f0cb8"
---
A GitHub login callback is refused before any code exchange, sending the browser back to the app with login_error=expired, unless its state matches the state cookie set when the login began.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A callback with state abc and a state cookie of other is sent back with login_error=expired.
