---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T17:57:56.233858Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/purge.go#L227-243"
    excerpt_sha256: "sha256:2803a6487fe5c552688adbe604e5788c961d9bd4051cf1c7124b6c81c992ec83"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L145-176"
    excerpt_sha256: "sha256:660bc6617bfbc7095caa073c849c93e68fe2718cba55af30a679b19d50caaf5d"
---
An account's stored files are removed before its database rows are erased, and the rows are erased in one transaction, so a failure leaves rows pointing at missing files that the next sweep can finish, never a file that no row names.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- If the object store fails mid-purge, the account stays claimed and the next sweep retries the deletion.
