---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/workspace/purge.go#L47-63"
    excerpt_sha256: "sha256:d588cbf6256bba6e28af204b6b7465dbf9cd229c111fe73349df0f3a5f3b4f15"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L78-99"
    excerpt_sha256: "sha256:b2cddd12981e24193eb23240c8570879ad52e8cb7756935e9233f978188ca0c3"
---
Purging a deleted account runs the purge step of every Context that keeps data for its workspace (analytics, testlab, run, packaging, creation, eval, publishing, registry, ingest) together with the object listings of testlab, run and packaging; when any of them is missing, the purge refuses to start instead of purging part of the account.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- An account past its grace period has its graders, creation sessions, publications and skills removed in one purge.
- A deployment wired without the eval purge step refuses every account purge and names eval as missing.
- Registry's step runs before ingest's, so import sources no remaining version points at are removed in the same purge.
