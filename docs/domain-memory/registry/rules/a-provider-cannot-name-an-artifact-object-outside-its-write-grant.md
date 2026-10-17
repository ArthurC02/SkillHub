---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T18:18:26.030858Z"
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L665-692"
    excerpt_sha256: "sha256:53c14820b6b09819c350716c621b2ff693ae43e6eafe2499b51a4133ea80fabc"
  - resource: "apps/platform/internal/trial/execution/provider_response_test.go#L18-29"
    excerpt_sha256: "sha256:8c8cc7dadd470111ef418fed96278c87af5ce936c21ae0cb4c1aa0ec975d608e"
---
An artifact the provider reports may name only the archive object the platform derived for the attempt; any other object key is refused.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A manifest entry naming another Workspace's dataset object is refused as outside its write grant.
