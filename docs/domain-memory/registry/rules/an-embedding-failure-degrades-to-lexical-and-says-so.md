---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-12T07:04:32.471255Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/service.go#L233-243"
    excerpt_sha256: "sha256:90dcd73cbcedbcd93410a52773c1fb7a625c1254156e6788323917a0700d1aa9"
  - resource: "apps/platform/internal/skill/discovery/service.go#L287-308"
    excerpt_sha256: "sha256:c6ea4d3aa51886fdf7242d8672bd7c51482707cca892a6e8252885b9de3e754a"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/disc_integration_test.go#L704-738"
    excerpt_sha256: "sha256:d33fe60fd0b2462807a1b13c8c1fac85e3341e88c89cfa2542822bf8a2808ab4"
---
When embeddings are missing or fail, search returns the lexical results and says it is degraded, rather than failing the request.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- With the embedding endpoint returning 502, a lexically matching skill is still returned with degraded set and a reason.
