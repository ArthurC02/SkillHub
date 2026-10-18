---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T06:01:37.914061Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/download.go#L159-170"
    excerpt_sha256: "sha256:54ca36308fa1e4a0e775cdecbb7c36ac1ffd7c6e0dc0275bedf51718235f0f63"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/download_integration_test.go#L699-722"
    excerpt_sha256: "sha256:72a92157ee1237a85f6668fbe0e9d9139ba80b08119c3004fdac1c4b1503abee"
---
A download artifact is read by its id together with the asking Workspace, so another Workspace's artifact can be neither seen nor fetched.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A stranger asking for someone else's download gets 404.
