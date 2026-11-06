---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-06T19:38:45.654816Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/evaluation_test.go#L209-255"
    excerpt_sha256: "sha256:f7916b8d47b08f9157c7f24573e676ad5feef8643653e0ad00a99355a101bf7d"
  - resource: "apps/platform/internal/trial/improvement/evaluation.go#L271-277"
    excerpt_sha256: "sha256:d582318163f672a2fb451747e123eaf55ca7d37ea7dcd81d73f1bf6377b83533"
---
A suggestion decision must be accepted or rejected; pending or any other word is refused as not_a_choice.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Deciding maybe, or going back to pending, leaves the decision as it was.
