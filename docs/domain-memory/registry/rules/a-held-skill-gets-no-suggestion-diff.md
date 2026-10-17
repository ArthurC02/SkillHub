---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:49:07.637777Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L165-169"
    excerpt_sha256: "sha256:c6d874d6357a03f12bd1b654b9f430ba9d3b2e5b311a3088a913683298ace043"
  - resource: "apps/platform/internal/trial/improvement/suggest_test.go#L21-28"
    excerpt_sha256: "sha256:64c779f71e0122412cd2ac982ed6666842ec4b14ce1db4250852e12cf5883d4d"
---
A Skill held for a licensing question gets no reproduced diff and no version from its suggestions; the block is decided before any file is read.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A held Skill's suggestion preview is blocked as access_restricted.
