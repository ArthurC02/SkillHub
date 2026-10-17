---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/product/operations/proposals.go#L61-67"
    excerpt_sha256: "sha256:2fbf91fa908148ac1b5ae4d2b37499609071c611ba7d52bdc964eac461a4f1a3"
---
A platform agent conclusion that proposes an action outside its definition's action list fails as a whole, so nothing is recorded.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A daily-report agent not allowed run-purge-audit that proposes it fails the run.
