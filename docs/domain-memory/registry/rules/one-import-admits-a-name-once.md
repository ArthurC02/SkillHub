---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T04:35:41.095408Z"
sources:
  - resource: "apps/platform/internal/skill/admission/source.go#L84-102"
    excerpt_sha256: "sha256:1eed6f0e19d93b7c23c49fa1495b4f9d2131ad738f563dd3fd522d6d5b43dce4"
  - resource: "apps/platform/internal/skill/admission/source_test.go#L250-284"
    excerpt_sha256: "sha256:18448ef607d72580c1f28e12014d135bbbbd309df87edcbfa25ffd942a85ee1c"
---
When two directories of one source declare the same Skill name, the first keeps it and the later ones are refused with a duplicate-name finding naming the first path, instead of silently becoming its next version.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- skills/alpha and skills/beta both named alpha: alpha is admitted, beta is refused.
