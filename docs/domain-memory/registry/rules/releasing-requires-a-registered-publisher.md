---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L284-291"
    excerpt_sha256: "sha256:0fd744d3c2f6d067153444e3d5f87ab5b21e10735019f8630f3164cca18ee010"
  - resource: "apps/platform/internal/skill/publishing/lock_characterization_test.go#L69-88"
    excerpt_sha256: "sha256:f354f1635271078e0591b08009e7c33cb05e24541c3be4532028aef393104774"
---
A Workspace without a registered publisher name cannot release; the release is refused with ErrNoPublisher.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- A Workspace with no publishers row is refused as no_publisher.
