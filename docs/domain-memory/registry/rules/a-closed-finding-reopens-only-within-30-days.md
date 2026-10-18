---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:59:29.446208Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:54.156118Z"
sources:
  - resource: "apps/platform/internal/product/operations/findings.go#L48-48"
    excerpt_sha256: "sha256:b876a3f74ff31aba1ff3e06435e11cf67f4cbd8afbc43c009a3733a3f3d41995"
  - resource: "apps/platform/internal/product/operations/findings.go#L122-124"
    excerpt_sha256: "sha256:4f2970c1633d476def9f3407031c28b2f07dab2cd7dd1f6fce76a5eaa8c60fdf"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agent_findings_integration_test.go#L289-319"
    excerpt_sha256: "sha256:1ef713268eb446d182046a0607517cc51eb2f8b9789b95768a3d9a81626c8749"
---
A new sighting reopens a closed finding only when it was last seen within 30 days; after that it opens a new finding.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A finding resolved and last seen 30 days and an hour ago stays resolved and a second finding opens.
