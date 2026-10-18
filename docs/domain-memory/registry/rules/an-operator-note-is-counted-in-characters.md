---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:52:30.207485Z"
sources:
  - resource: "apps/platform/internal/skill/library/governance/restriction.go#L142-151"
    excerpt_sha256: "sha256:4da7785316343e162a5b48d76ab76341163c87fcaa6bb1bb8ba038c26f90b0f2"
  - resource: "apps/platform/internal/skill/library/governance/restriction_test.go#L92-99"
    excerpt_sha256: "sha256:61a653aaba4319979d49239f7749308769e82fff2e73c0ad52124f8b04e62657"
---
The operator note cap counts characters, not bytes, so a note of Chinese characters up to the cap is accepted.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A note of exactly the cap in Chinese characters is accepted; one more is refused.
