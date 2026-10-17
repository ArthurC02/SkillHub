---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/evaluation.go#L178-184"
    excerpt_sha256: "sha256:e04c888239d2bd639dac10d1fb70cb3f493a4d2ecc4b6cb2764d10fbf0e24f21"
---
A suggestion decision must be accepted or rejected; pending or any other word is refused as not_a_choice.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Deciding maybe, or going back to pending, leaves the decision as it was.
