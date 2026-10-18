---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/delivery/download.go#L159-170"
    excerpt_sha256: "sha256:54ca36308fa1e4a0e775cdecbb7c36ac1ffd7c6e0dc0275bedf51718235f0f63"
---
A download artifact is read by its id together with the asking Workspace, so another Workspace's artifact can be neither seen nor fetched.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A stranger asking for someone else's download gets 404.
