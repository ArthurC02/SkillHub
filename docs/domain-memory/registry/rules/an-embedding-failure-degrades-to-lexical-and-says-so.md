---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/discovery/service.go#L231-241"
    excerpt_sha256: "sha256:90dcd73cbcedbcd93410a52773c1fb7a625c1254156e6788323917a0700d1aa9"
  - resource: "apps/platform/internal/skill/discovery/service.go#L286-307"
    excerpt_sha256: "sha256:c6ea4d3aa51886fdf7242d8672bd7c51482707cca892a6e8252885b9de3e754a"
---
When embeddings are missing or fail, search returns the lexical results and says it is degraded, rather than failing the request.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- With the embedding endpoint returning 502, a lexically matching skill is still returned with degraded set and a reason.
