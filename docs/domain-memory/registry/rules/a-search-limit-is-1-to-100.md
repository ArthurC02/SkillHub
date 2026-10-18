---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/discovery/http.go#L247-257"
    excerpt_sha256: "sha256:7df1f2f1567b02af421b590a1d0806e1822000e60c2e706d9d48f452e6363800"
---
The search limit must be a whole number from 1 to 100; anything else is refused.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- limit=100 is accepted and limit=101 answers 400.
