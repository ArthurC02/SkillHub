---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T09:26:27.855880Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/suggest.go#L88-91"
    excerpt_sha256: "sha256:85ba48174c542f26f84bccf90af9a35dbad8d47c9553bf114cd392c234ccda6d"
  - resource: "apps/platform/internal/trial/improvement/suggest_store_test.go#L68-91"
    excerpt_sha256: "sha256:bcb532ca76e65f5584a0db7bc6ce6cd65d193490a2fdb5e8d6a85c16ee1bb677"
---
At most ten suggestions are stored for one evaluation; the rest are counted as dropped over the cap.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Twelve citable proposals store ten.
