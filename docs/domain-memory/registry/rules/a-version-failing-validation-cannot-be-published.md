---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T02:50:56.766681Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/service.go#L201-207"
    excerpt_sha256: "sha256:1000826da41f85768807e10f752adacc4e4b8659cd8a27375845a31f2c5f6832"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/publishing_integration_test.go#L130-150"
    excerpt_sha256: "sha256:3500279e08e37b922c3ebb8bb743409585db09d31e5e4a98198ac375458d4ed0"
---
Publishing scans the version's stored package and refuses as validation_blocked when it finds any validation error; no publication is recorded.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- A version carrying a credential in .env answers 422 validation_blocked and leaves no publication row.
