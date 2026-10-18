---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/discovery/fusion.go#L54-71"
    excerpt_sha256: "sha256:092c04eefb8f53f91778d3b610a932194b07b94e4efde0391ecfc6dc586d6d1e"
---
Hybrid results are ordered exact name first, then lexically covered, then vector-ranked by nearest distance, with unranked last and the skill id as the final tiebreak.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- A skill named exactly as the query outranks a covered skill, which outranks a nearer uncovered one.
