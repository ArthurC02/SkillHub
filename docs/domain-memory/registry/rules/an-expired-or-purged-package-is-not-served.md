---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/delivery/download.go#L208-210"
    excerpt_sha256: "sha256:d43408a8449614683e3c4c6f52c1369828da2b270d7a0857288af091184ce342"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/download_integration_test.go#L840-878"
    excerpt_sha256: "sha256:ec3eb6efb17f471fece2534707aa50b64200147b5f4767db5f1b76714f0e374c"
---
A package is not served once it has expired, been purged or is not scan-available.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- Fetching an expired download answers gone.
