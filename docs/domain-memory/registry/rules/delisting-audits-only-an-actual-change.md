---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:06:36.790775Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L402-418"
    excerpt_sha256: "sha256:0e0c7d4ed44015bf994aa6a5cb2065683e44cd4de86a20903db64bc3ad1394d8"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/publishing_release_guard_integration_test.go#L44-64"
    excerpt_sha256: "sha256:79c69f97fc71d3ebc1bcc07565428d1f5966f48fa5fec8a514b5ee782a6aeb3a"
---
Delisting writes an audit event only when it changes the publication, so delisting twice records one delist.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- Delisting an already delisted publication adds no second audit event.
