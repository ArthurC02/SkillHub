---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-31T06:34:02.458664Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/github.go#L161-175"
    excerpt_sha256: "sha256:19db2141c1569a717bf1fc95f3db30eed8ceb2e828ab1b63c7aa45b59ed9dd46"
  - resource: "apps/platform/internal/creator/workspace/github_test.go#L212-239"
    excerpt_sha256: "sha256:ce9f79d0ec6c7be9792f74719c7a6e63f74c1a562b2b7992e2e1609db581de5f"
  - resource: "apps/platform/internal/creator/workspace/http.go#L687-692"
    excerpt_sha256: "sha256:434c1e0125fb07d5a3c548e52d4b2a883da26f436037c0ec8c4548d0026c8983"
---
GitHub rate limiting, server errors and transport failures send the browser back to the app with login_error=github_unavailable, while other GitHub refusals send github_refused, so an outage never reads as a bad credential.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- GitHub answering 429 on the token exchange sends the browser back with login_error=github_unavailable.
