---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L365-382"
    excerpt_sha256: "sha256:7b3500f9ce846197bcfd87fa53339199ff7ca5c53ee6cff128183c379665d253"
  - resource: "apps/platform/internal/creator/credit/service_test.go#L398-419"
    excerpt_sha256: "sha256:ef5d04abbe1616793a579ae5cc1cd81c9ecd8266a93a826aef11d4c37e55a006"
---
Replaying a grant under the same key returns the current balance and writes no second audit event.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- Granting twice with the same key returns 50 both times and writes one audit event.
