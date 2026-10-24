---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/creation/service.go#L87-89"
    excerpt_sha256: "sha256:85bfa24839cfc3a63ecedff48c06a1ad2e8890426b8c8091b780a7ac2426cf76"
  - resource: "apps/platform/internal/creator/creation/limits_retention_test.go#L8-26"
    excerpt_sha256: "sha256:538dbabc0e0fbd9ce7aece4d83c4f0a1760ae4c610405be82bcccb7401235665"
---
Creation limits are usable only when the retention outlasts the session timeout plus the longest step that can start just before it: the call timeout, ten seconds of slack and sixty seconds to revoke the key, search ahead and settle; otherwise a session row could be deleted before its last step's spend is charged.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- With a one-minute session and a one-second call timeout, a retention of one minute and seventy-one seconds is usable and one second less is not.
