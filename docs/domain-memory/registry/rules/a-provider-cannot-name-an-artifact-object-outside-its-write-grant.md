---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L665-692"
    excerpt_sha256: "sha256:53c14820b6b09819c350716c621b2ff693ae43e6eafe2499b51a4133ea80fabc"
---
An artifact the provider reports may name only the archive object the platform derived for the attempt; any other object key is refused.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A manifest entry naming another Workspace's dataset object is refused as outside its write grant.
