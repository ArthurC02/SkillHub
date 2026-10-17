---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L165-169"
    excerpt_sha256: "sha256:c6d874d6357a03f12bd1b654b9f430ba9d3b2e5b311a3088a913683298ace043"
---
A Skill held for a licensing question gets no reproduced diff and no version from its suggestions; the block is decided before any file is read.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A held Skill's suggestion preview is blocked as access_restricted.
