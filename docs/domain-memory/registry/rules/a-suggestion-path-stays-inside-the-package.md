---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L228-238"
    excerpt_sha256: "sha256:8d29a5dda5e6be22e6ac2413293cf8231513265f4798fa1215af7436b21a07d6"
---
A suggestion's target must be a clean relative path inside the Skill package; empty, absolute, traversing or uncleaned paths are refused, not repaired.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A proposal targeting ../../etc/passwd or ./SKILL.md is refused.
