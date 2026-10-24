---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/activity.go#L36-43"
    excerpt_sha256: "sha256:bccd750657145b2ee6609f8059666e0b1f489c6411d2d53349744d4b788473fe"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/activity_integration_test.go#L137-145"
    excerpt_sha256: "sha256:76c4f331def1c7138ff0e571926a1076891edba0dde252f5c11bb1b735cceda4"
---
The activity feed refuses a page limit outside 1 to 100 with 400.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- limit=0 and limit=101 both answer 400.
