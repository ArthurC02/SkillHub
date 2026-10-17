---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/github.go#L124-142"
    excerpt_sha256: "sha256:9674ee19536ccb9a0d165569d198ebbc02ef8432870c20a1998c83561d1aba1c"
---
When the GitHub profile carries no email, the login uses only the address GitHub marks both primary and verified.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- Of a verified old address and a primary verified one, the primary is used.
