---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:49:07.637777Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/evaluation.go#L147-152"
    excerpt_sha256: "sha256:5aa83b79579c4f64f96680c2c0b428519540d35b8b503c0db000e49e35432e37"
  - resource: "apps/platform/internal/trial/improvement/evaluation_test.go#L152-189"
    excerpt_sha256: "sha256:c41fab7e4e26b585f07e5c98b5643869130397a4aa052cc2c12a77ac69b95e9e"
---
An evaluation settles only while it awaits the judge; a second verdict on a settled evaluation is refused, so the first stands.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Completing an already failed evaluation leaves it failed.
