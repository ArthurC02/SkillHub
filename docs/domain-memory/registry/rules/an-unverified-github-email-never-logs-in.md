---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/github.go#L133-142"
    excerpt_sha256: "sha256:93ed9e3b5f4393880b3a552f0e327ef9d93d5ba9b081ca0eb4ca8f29023f0b2d"
---
A GitHub login whose profile carries no email is refused when no address is both primary and verified, so an unverified address can never claim an account.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A GitHub account whose only primary address is unverified is refused.
