---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/delivery/activity.go#L46-57"
    excerpt_sha256: "sha256:7e629e0358e1879cb6017430ffe27f7deaf54ac7ccf669393741b803ee135abf"
  - resource: "apps/platform/internal/skill/delivery/activity_test.go#L9-21"
    excerpt_sha256: "sha256:c89efbf323399f0f3e3e90221230188fca8500f04a58472ee50c380839897d1c"
---
A download artifact's activity item is recent only once its scan made it available; a quarantined, rejected or unknown scan state asks for attention.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A quarantined package shows 套件仍在隔離區 as needs attention.
