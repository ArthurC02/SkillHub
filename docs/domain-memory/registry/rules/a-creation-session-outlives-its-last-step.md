---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T22:26:21.075638Z"
sources:
  - resource: "apps/platform/internal/creator/creation/service.go#L89-91"
    excerpt_sha256: "sha256:9e2a6e89e2413532d8c3ef10a6f98bf138ce42695ae2ea4e2771d9098224b130"
  - resource: "apps/platform/internal/creator/creation/limits_retention_test.go#L8-26"
    excerpt_sha256: "sha256:538dbabc0e0fbd9ce7aece4d83c4f0a1760ae4c610405be82bcccb7401235665"
---
Creation limits are usable only when the retention outlasts the session timeout plus the longest step that can start just before it: the call timeout, ten seconds of slack and sixty seconds to revoke the key, search ahead and settle; otherwise a session row could be deleted before its last step's spend is charged.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- With a one-minute session and a one-second call timeout, a retention of one minute and seventy-one seconds is usable and one second less is not.
