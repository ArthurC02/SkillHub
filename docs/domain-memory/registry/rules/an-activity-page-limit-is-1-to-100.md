---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/activity.go#L36-43"
    excerpt_sha256: "sha256:bccd750657145b2ee6609f8059666e0b1f489c6411d2d53349744d4b788473fe"
---
The activity feed refuses a page limit outside 1 to 100 with 400.

# Relations
- Contexts: [activity](/contexts/activity)

# Examples
- limit=0 and limit=101 both answer 400.
