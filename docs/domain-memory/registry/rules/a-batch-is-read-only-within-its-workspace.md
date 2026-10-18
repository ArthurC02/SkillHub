---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L304-313"
    excerpt_sha256: "sha256:bd1d7e3044d606e40203e3bb190cdc916ee6d33610fdf7d1f9ed3c167720e618"
---
A batch is read by its id together with the asking Workspace, so another Workspace's batch reads as not found.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A stranger asking for someone else's batch report gets 404.
