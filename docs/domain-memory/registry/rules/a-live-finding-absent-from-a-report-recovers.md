---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:14:38.912433Z"
sources:
  - resource: "apps/platform/internal/product/operations/findings.go#L149-155"
    excerpt_sha256: "sha256:3c9d463a383b8581258e7487a137f68103046907f373328e4c240550a5d1acdd"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agent_findings_integration_test.go#L144-171"
    excerpt_sha256: "sha256:d468bc3dce726f0f3b22d777df31afe8ce7599a8e0d7217846ed6576ad5f19de"
---
After a successful report, every open or acknowledged finding the report did not cite becomes recovered, and reopens if a later report cites it.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A stuck-queue finding absent from today's report becomes recovered.
