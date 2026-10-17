---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T17:11:22.999935Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/github.go#L161-175"
    excerpt_sha256: "sha256:19db2141c1569a717bf1fc95f3db30eed8ceb2e828ab1b63c7aa45b59ed9dd46"
  - resource: "apps/platform/internal/creator/workspace/github_test.go#L211-238"
    excerpt_sha256: "sha256:5fbccf129a0883aaf9e689f81ea09d2366135c86a936ec227e8e4b9730b3178c"
---
GitHub rate limiting, server errors and transport failures answer 503, while other GitHub refusals answer 401, so an outage never reads as a bad credential.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- GitHub answering 429 on the token exchange makes the callback answer 503.
