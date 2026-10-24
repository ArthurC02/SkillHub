---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L203-209"
    excerpt_sha256: "sha256:1000826da41f85768807e10f752adacc4e4b8659cd8a27375845a31f2c5f6832"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/publishing_integration_test.go#L133-153"
    excerpt_sha256: "sha256:3500279e08e37b922c3ebb8bb743409585db09d31e5e4a98198ac375458d4ed0"
---
Publishing scans the version's stored package and refuses as validation_blocked when it finds any validation error; no publication is recorded.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- A version carrying a credential in .env answers 422 validation_blocked and leaves no publication row.
