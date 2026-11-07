---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T15:07:58.144675Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/evaluation_test.go#L209-255"
    excerpt_sha256: "sha256:095fabba90ccd7e0c23410b922b7988e4c66a4c810fdf4f55fc37470ca9154cc"
  - resource: "apps/platform/internal/trial/improvement/evaluation.go#L271-277"
    excerpt_sha256: "sha256:d582318163f672a2fb451747e123eaf55ca7d37ea7dcd81d73f1bf6377b83533"
---
A suggestion decision must be accepted or rejected; pending or any other word is refused as not_a_choice.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Deciding maybe, or going back to pending, leaves the decision as it was.
