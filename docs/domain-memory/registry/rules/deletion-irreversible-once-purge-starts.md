---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/account.go#L42-69"
    excerpt_sha256: "sha256:f299a7a5956ac36a9374f06b675c2cf5b0a10b91aa65a2ff4c2b9aa2f8ca95f8"
  - resource: "apps/platform/internal/creator/workspace/http.go#L611-641"
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
