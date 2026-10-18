---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T09:26:27.855880Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/suggest.go#L187-195"
    excerpt_sha256: "sha256:64284e90acc16f59b195d0fac8bf1736d25a8bcf733e58297bfaad3590e95feb"
  - resource: "apps/platform/internal/trial/improvement/suggest_test.go#L137-170"
    excerpt_sha256: "sha256:0b9735a84efbd2a96dfcaebd5ac1a0d96d3f6c45b2e6e194da60fdc95a2b2365"
---
A quote shorter than twelve characters cannot be matched to a verified excerpt as suggestion evidence.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Evidence "e" inside a verified excerpt is refused.
