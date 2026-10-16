---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:32:08.394666Z"
sources:
  - resource: "apps/platform/internal/trial/execution/reassign.go#L73-77"
    excerpt_sha256: "sha256:9a952ee3fef2b37747a2894803666dbb6775e370c1c9a17611bc0248289b34e8"
  - resource: "apps/platform/internal/trial/execution/reassign.go#L92-103"
    excerpt_sha256: "sha256:60410d86ddd74fc448481e494a1bb8e8cc52b17032388de8a6f3fa452e0b0547"
---
When a Run's attempt is lost with its provider, the platform asks the model gateway to revoke that attempt's model key before it dispatches the next attempt, so the lost sandbox cannot keep spending on a key the Run no longer uses. A revocation the gateway refuses is counted and left to cleanup, which revokes it again.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A provider that stops answering mid-run: the attempt ends provider_lost, its key is revoked, and a new attempt receives a new key.
- The gateway is unreachable at revocation time: the reassignment still proceeds and the cleanup pass revokes the key later.
