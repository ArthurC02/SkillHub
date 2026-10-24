---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L345-354"
    excerpt_sha256: "sha256:bd1d7e3044d606e40203e3bb190cdc916ee6d33610fdf7d1f9ed3c167720e618"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/authz_stranger_matrix_integration_test.go#L400-450"
    excerpt_sha256: "sha256:180e6d234429ccc17ef69786ec69cb0f51b06a1a87f78aa68e3684f1e9c1928e"
---
A batch is read by its id together with the asking Workspace, so another Workspace's batch reads as not found.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A stranger asking for someone else's batch report gets 404.
