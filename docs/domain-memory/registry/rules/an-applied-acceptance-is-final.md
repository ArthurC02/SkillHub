---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T15:07:58.144675Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/evaluation_test.go#L209-255"
    excerpt_sha256: "sha256:095fabba90ccd7e0c23410b922b7988e4c66a4c810fdf4f55fc37470ca9154cc"
  - resource: "apps/platform/internal/trial/improvement/evaluation.go#L278-279"
    excerpt_sha256: "sha256:00f4885c3c736f5cc30fce59ad00bb6007ce49f3b9720a625fd37adef39d40de"
---
Once a suggestion has gone into a Skill version its acceptance cannot be withdrawn; rejecting it is refused as acceptance_is_final.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A suggestion applied to version 9 cannot be flipped to rejected.
