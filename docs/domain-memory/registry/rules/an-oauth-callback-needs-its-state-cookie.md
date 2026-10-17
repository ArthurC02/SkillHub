---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/http.go#L120-124"
    excerpt_sha256: "sha256:6a570ea78e1c8c2a9058de2411a3e61f5b45c3ebb397084a1d0feda7a90b7ced"
---
A GitHub login callback is refused with 401 before any code exchange unless its state matches the state cookie set when the login began.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A callback with state abc and a state cookie of other answers 401.
