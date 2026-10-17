---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/operations/proposals.go#L91-97"
    excerpt_sha256: "sha256:89994fa5c32ede52796e9d47871dc32d689fab8ca202951e16cb11618db2d526"
---
A new proposal is skipped while another proposal for the same action is proposed, approved or running.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- Two daily reports proposing run-purge-audit leave one proposal until the first closes.
