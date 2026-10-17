---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L402-418"
    excerpt_sha256: "sha256:0e0c7d4ed44015bf994aa6a5cb2065683e44cd4de86a20903db64bc3ad1394d8"
---
Delisting writes an audit event only when it changes the publication, so delisting twice records one delist.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Delisting an already delisted publication adds no second audit event.
