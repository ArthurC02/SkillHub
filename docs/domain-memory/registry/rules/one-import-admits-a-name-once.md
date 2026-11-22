---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-22T17:35:01.950480Z"
sources:
  - resource: "apps/platform/internal/skill/admission/source.go#L84-102"
    excerpt_sha256: "sha256:1eed6f0e19d93b7c23c49fa1495b4f9d2131ad738f563dd3fd522d6d5b43dce4"
  - resource: "apps/platform/internal/skill/admission/source_test.go#L250-283"
    excerpt_sha256: "sha256:9e74865fda7a04f0a588b0989424398871fc2622213c7d69bca860f066113cf8"
---
When two directories of one source declare the same Skill name, the first keeps it and the later ones are refused with a duplicate-name finding naming the first path, instead of silently becoming its next version.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- one/alpha and two/alpha both named alpha: the first is admitted, the second refused.
- Two folders cannot collide by declaring a name neither folder carries: such a package is refused for its folder name before the duplicate is reached.
