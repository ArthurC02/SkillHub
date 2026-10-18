---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/discovery/fusion.go#L36-48"
    excerpt_sha256: "sha256:94668aed79c9c35f057e4485aaa9b25dd517faf1578558affc555014bb3dcf32"
  - resource: "apps/platform/internal/skill/discovery/service.go#L227-230"
    excerpt_sha256: "sha256:22a810c83091544767287987df1794cc07bbbd15ffadcc2151961db566fa1706"
---
A hybrid candidate farther than the cosine distance cutoff is dropped unless the lexical leg covered it or it has no vector.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- A skill just past the cutoff that matches no query word is excluded; one that matches the words is kept.
