---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/library/governance/restriction.go#L142-151"
    excerpt_sha256: "sha256:4da7785316343e162a5b48d76ab76341163c87fcaa6bb1bb8ba038c26f90b0f2"
  - resource: "apps/platform/internal/skill/library/governance/restriction_test.go#L86-90"
    excerpt_sha256: "sha256:36ef5ab036e207f1e17a4b797bd5fd4994bdb538265c550da897b8dc3aedad9c"
---
An operator note longer than the contract's cap is refused; a note of exactly the cap is accepted.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A takedown note one character over the cap answers 400.
