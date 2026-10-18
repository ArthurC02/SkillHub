---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T06:26:10.741232Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/fusion.go#L54-71"
    excerpt_sha256: "sha256:092c04eefb8f53f91778d3b610a932194b07b94e4efde0391ecfc6dc586d6d1e"
  - resource: "apps/platform/internal/skill/discovery/fusion_test.go#L100-128"
    excerpt_sha256: "sha256:eb34e54f415faf6e7e7cf544341cda50c4301c2fb773c92bd1712337982693e9"
---
Hybrid results are ordered exact name first, then lexically covered, then vector-ranked by nearest distance, with unranked last and the skill id as the final tiebreak.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- A skill named exactly as the query outranks a covered skill, which outranks a nearer uncovered one.
