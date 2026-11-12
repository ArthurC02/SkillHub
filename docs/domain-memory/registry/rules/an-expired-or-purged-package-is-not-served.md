---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-12T09:34:36.964247Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/download.go#L210-212"
    excerpt_sha256: "sha256:d43408a8449614683e3c4c6f52c1369828da2b270d7a0857288af091184ce342"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/download_integration_test.go#L857-895"
    excerpt_sha256: "sha256:c55a2048a302d10a44148a436b6b88689b6275429cb22b1b0165d6bdba4979fd"
---
A package is not served once it has expired, been purged or is not scan-available.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- Fetching an expired download answers gone.
