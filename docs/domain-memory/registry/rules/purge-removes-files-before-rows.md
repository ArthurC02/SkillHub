---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/purge.go#L298-314"
    excerpt_sha256: "sha256:2803a6487fe5c552688adbe604e5788c961d9bd4051cf1c7124b6c81c992ec83"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L157-190"
    excerpt_sha256: "sha256:9e1bb6e3869b17c6f1f1596cd6251a7cff4f78209623b655c09501f3e8bcee4b"
---
An account's stored files are removed before its database rows are erased, and the rows are erased in one transaction, so a failure leaves rows pointing at missing files that the next sweep can finish, never a file that no row names.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- If the object store fails mid-purge, the account stays claimed and the next sweep retries the deletion.
