---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/account.go#L52-61"
    excerpt_sha256: "sha256:4887239e31f20b9e79c6f0567fc676a87360e0d526f47c979857128b70498dd8"
  - resource: "apps/platform/internal/creator/workspace/account_test.go#L34-71"
    excerpt_sha256: "sha256:e877185dc0ac36b0f6dd870720f86296ffcec89f918b1e66c652b157213cf838"
---
An operator may register an erasure for an account on its holder's behalf, with a reason; it requests deletion if none is pending, keeps an earlier request time, and lets the next purge sweep take the account without waiting out the 30-day grace. The holder cancelling the deletion withdraws it, and an account already purging or deleted refuses it.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- An account erased by an operator today is purged by today's sweep, while one deleted by its owner today waits; erasing a purged account answers 409.
