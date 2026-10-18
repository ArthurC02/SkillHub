---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/batch/batch_report.go#L29-29"
    excerpt_sha256: "sha256:14e4f3b8df6a1b982c4f5f8eababe38b1614ece72ff7b7104b3bf8c9874d484e"
  - resource: "apps/platform/internal/trial/batch/batch_test.go#L48-67"
    excerpt_sha256: "sha256:cb127ae8f3faac652aab33c76fbdffb9560c340386a2f22cc57d02c110d8a92f"
---
A pass rate carries a 95 percent Wilson score interval, and none when nothing is judged.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- Two passed of four judged gives about [0.150, 0.850].
