---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/github.go#L161-175"
    excerpt_sha256: "sha256:19db2141c1569a717bf1fc95f3db30eed8ceb2e828ab1b63c7aa45b59ed9dd46"
---
GitHub rate limiting, server errors and transport failures answer 503, while other GitHub refusals answer 401, so an outage never reads as a bad credential.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- GitHub answering 429 on the token exchange makes the callback answer 503.
