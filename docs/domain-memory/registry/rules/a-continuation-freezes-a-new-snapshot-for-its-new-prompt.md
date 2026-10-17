---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:46:14.276516Z"
sources:
  - resource: "apps/platform/internal/trial/design/snapshot.go#L295-345"
    excerpt_sha256: "sha256:ff95ff66ec683de274f24406cbb28a9a91f802c276625c01e0c646b8dcddf204"
  - resource: "apps/platform/internal/trial/design/snapshot_continue_integration_test.go#L8-48"
    excerpt_sha256: "sha256:0a8e43125f51d4eb86c0518a8ddd5364ad3b565296d5493e9fc97ddd21dff328"
---
Continuing a run freezes a new snapshot of the same Test Case whose content hash covers the new prompt, and leaves the earlier snapshot as it was.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- Continuing with now do it in French gives a new snapshot with a different hash; continuing with the same prompt gives the same hash.
