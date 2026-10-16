---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/account.go#L41-57"
    excerpt_sha256: "sha256:8ff1053d6d1caa19b5c0fe2476c207a4fb6a2d5b54a35f1561008d99f11d8808"
  - resource: "apps/platform/internal/creator/workspace/http.go#L550-580"
    excerpt_sha256: "sha256:694854a88d93cbfe6e3d820134e8fa46b0b7d9c256c54ffdf28c526e0271e58b"
  - resource: "contracts/openapi/public.yaml#L222-229"
    excerpt_sha256: "sha256:c0d734698f112f6a5b072328448d3e936105fd071bf5acf81d324c030b5725a0"
---
Once an Account has started purging or is deleted, deletion can neither be requested nor cancelled; the user is told it is irreversible (HTTP 409).

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- Cancelling a deletion whose purge has started answers 409 'deletion is already irreversible'.
- Requesting deletion for an already deleted Account answers 409.
