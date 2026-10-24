---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L379-396"
    excerpt_sha256: "sha256:7b3500f9ce846197bcfd87fa53339199ff7ca5c53ee6cff128183c379665d253"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L437-458"
    excerpt_sha256: "sha256:ef5d04abbe1616793a579ae5cc1cd81c9ecd8266a93a826aef11d4c37e55a006"
---
Replaying a grant under the same key returns the current balance and writes no second audit event.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Granting twice with the same key returns 50 both times and writes one audit event.
