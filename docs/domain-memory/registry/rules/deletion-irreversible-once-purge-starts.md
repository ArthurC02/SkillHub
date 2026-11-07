---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T22:26:21.075638Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/account.go#L42-69"
    excerpt_sha256: "sha256:f299a7a5956ac36a9374f06b675c2cf5b0a10b91aa65a2ff4c2b9aa2f8ca95f8"
  - resource: "apps/platform/internal/creator/workspace/http.go#L613-643"
    excerpt_sha256: "sha256:a96b3765e9f71383ac36f9a50c00330d6dca6bc52a8b048d3756199938d97444"
  - resource: "contracts/openapi/public.yaml#L222-229"
    excerpt_sha256: "sha256:c0d734698f112f6a5b072328448d3e936105fd071bf5acf81d324c030b5725a0"
---
Once an Account has started purging or is deleted, deletion can neither be requested nor cancelled; the user is told it is irreversible (HTTP 409).

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- Cancelling a deletion whose purge has started answers 409 'deletion is already irreversible'.
- Requesting deletion for an already deleted Account answers 409.
