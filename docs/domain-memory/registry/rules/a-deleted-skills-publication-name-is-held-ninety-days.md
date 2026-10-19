---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L360-362"
    excerpt_sha256: "sha256:32ad13772291d85b0fc9ae586b5fbca2d1b857c162db4c6d81454a0bcecd3006"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/publishing_integration_test.go#L424-469"
    excerpt_sha256: "sha256:abd321ff310cef8e91627c5a2c5e1509e896c48099f6edb426e51d5a0956dfc1"
---
When a published Skill is deleted its publication retires; for 90 days after that its old address still answers withdrawn and the publisher cannot give its name to another Skill; after 90 days the name passes to another of the same publisher's Skills and the address follows it.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Reusing the name 89 days 23 hours after the delete answers 409 name_held; 90 days and a minute after, it publishes and the old address serves the successor.
