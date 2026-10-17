---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/evaluation.go#L147-152"
    excerpt_sha256: "sha256:5aa83b79579c4f64f96680c2c0b428519540d35b8b503c0db000e49e35432e37"
---
An evaluation settles only while it awaits the judge; a second verdict on a settled evaluation is refused, so the first stands.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Completing an already failed evaluation leaves it failed.
