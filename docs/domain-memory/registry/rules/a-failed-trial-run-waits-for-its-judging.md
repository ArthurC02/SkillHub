---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T20:31:55.890891Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_report.go#L191-192"
    excerpt_sha256: "sha256:79f47126768499f2f7251956d9830c83b2e9e82a2fb835aa2863d2b264364d9e"
  - resource: "apps/platform/internal/trial/batch/batch_test.go#L183-214"
    excerpt_sha256: "sha256:037c9b8fdb16d0c498ffa2b18a83248df9ba8a5609323c405a96e8fbf6baa4f0"
---
A trial whose run failed reads as running while its judging is under way, rather than as not evaluated.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A failed run with a judgement in progress reads running.
