---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/evaluation.go#L184-185"
    excerpt_sha256: "sha256:d247f84a2b5fbb66e4b7d469c8864454b619a894f3d3089b7fa6baba42099b3a"
---
Once a suggestion has gone into a Skill version its acceptance cannot be withdrawn; rejecting it is refused as acceptance_is_final.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A suggestion applied to version 9 cannot be flipped to rejected.
