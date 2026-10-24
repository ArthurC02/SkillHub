---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/activity.go#L60-60"
    excerpt_sha256: "sha256:fe57b8bf19df5a95d5f113c5409586c564371c4bbbb3897fb496a1fe459e5e0b"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/activity_integration_test.go#L114-135"
    excerpt_sha256: "sha256:15366a4eb994916d8e0c05ded6ed814aba72aaf63286aa48ad9c56ab8d6831a2"
---
When any activity source cannot be read, the request answers 503 with complete=false, the failed sources and no items.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- A failing run reader gives 503 naming runs.
