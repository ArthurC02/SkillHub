---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T06:26:10.741232Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/service.go#L273-275"
    excerpt_sha256: "sha256:c0ca0a351bb9ea1bb585cb2242a18aa3603166a0f5d2863d311b867ced9bfd77"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/search_purpose_integration_test.go#L11-37"
    excerpt_sha256: "sha256:61dd56f74c4a9a7e679788800e20ade35434bb9e397891e1e7f5d53a9b3bd58d"
---
A search made for reference records no search_performed analytics event.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- A purpose=reference search writes no search_performed row; the same search without purpose writes one.
