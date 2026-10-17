---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:49:07.637777Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L228-238"
    excerpt_sha256: "sha256:8d29a5dda5e6be22e6ac2413293cf8231513265f4798fa1215af7436b21a07d6"
  - resource: "apps/platform/internal/trial/improvement/suggest_test.go#L81-96"
    excerpt_sha256: "sha256:099aa14f57a807c343b0e1199f0bcedcd623e6ed7983c078e167a417e284b536"
---
A suggestion's target must be a clean relative path inside the Skill package; empty, absolute, traversing or uncleaned paths are refused, not repaired.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A proposal targeting ../../etc/passwd or ./SKILL.md is refused.
