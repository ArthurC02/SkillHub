---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T19:06:36.790775Z"
sources:
  - resource: "apps/platform/internal/skill/publishing/http.go#L150-150"
    excerpt_sha256: "sha256:219bae0a6b69660694667645c9f09403a77c7ec638131346d5617e3abeaf709b"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/publishing_body_cap_integration_test.go#L8-33"
    excerpt_sha256: "sha256:00a6e0624352217fb2334d7f502d520060eda438503c19b55a0461df092bebf5"
---
Every publishing request reads at most 64 KiB of JSON and refuses a longer body with 400.

# Relations
- Contexts: [publishing](/contexts/publishing)

# Examples
- A body of 64 KiB of spaces followed by valid JSON is refused.
