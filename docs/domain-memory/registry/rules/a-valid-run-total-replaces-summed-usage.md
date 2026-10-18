---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/evidence/general.go#L90-129"
    excerpt_sha256: "sha256:b773dee9bc656cd7cecd64fef67b4709d09df8feb3e8e3e13cefd94e600da479"
  - resource: "apps/platform/internal/trial/evidence/general_test.go#L115-153"
    excerpt_sha256: "sha256:8e814ce9395d34cb1bfd5954ff613149f399996e4d8fb0c53e91eeb6a60b2313"
---
Reported usage takes a readable run_total figure field by field and otherwise sums the per-call figures.

# Relations
- Contexts: [trace](/contexts/trace)

# Examples
- A run_total of 900 tokens is reported instead of the 1000 summed from its calls.
