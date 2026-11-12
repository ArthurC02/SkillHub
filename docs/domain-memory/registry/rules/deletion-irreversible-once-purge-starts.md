---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-11T17:10:17.837079Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/account.go#L42-69"
    excerpt_sha256: "sha256:f299a7a5956ac36a9374f06b675c2cf5b0a10b91aa65a2ff4c2b9aa2f8ca95f8"
  - resource: "apps/platform/internal/creator/workspace/http.go#L622-652"
    excerpt_sha256: "sha256:bd92e0ea8db5cf6297e4257620abec79e700086cd52ff5e79b29c7433583b3be"
  - resource: "contracts/openapi/public.yaml#L222-229"
    excerpt_sha256: "sha256:c0d734698f112f6a5b072328448d3e936105fd071bf5acf81d324c030b5725a0"
---
Once an Account has started purging or is deleted, deletion can neither be requested nor cancelled; the user is told it is irreversible (HTTP 409).

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- Cancelling a deletion whose purge has started answers 409 'deletion is already irreversible'.
- Requesting deletion for an already deleted Account answers 409.
