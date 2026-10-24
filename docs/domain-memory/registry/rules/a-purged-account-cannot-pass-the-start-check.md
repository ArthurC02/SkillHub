---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/credit/service.go#L205-207"
    excerpt_sha256: "sha256:a4f6aaec98f88c561d62415cb395499753cc6d506dd5ae367ca7c33e4c268e3d"
  - resource: "apps/platform/internal/creator/credit/refusals_test.go#L55-76"
    excerpt_sha256: "sha256:a34bc54f9b7c67a2cf3195059266049efa01990b9ae17777d84379ab70d4754d"
---
The start check refuses an account that account facts report missing or purged, before reading any balance.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- CanStart for a purged user returns ErrAccountGone.
