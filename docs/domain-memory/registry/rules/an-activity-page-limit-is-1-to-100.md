---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-08T08:46:39.695261Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/activity.go#L38-42"
    excerpt_sha256: "sha256:3c5eddca3f0f3f0f75bf5ad0860459ad919cde0a03a3ed793e71ec9db642b270"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/activity_integration_test.go#L137-145"
    excerpt_sha256: "sha256:76c4f331def1c7138ff0e571926a1076891edba0dde252f5c11bb1b735cceda4"
---
The activity feed refuses a page limit outside 1 to 100 with 400.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- limit=0 and limit=101 both answer 400.
