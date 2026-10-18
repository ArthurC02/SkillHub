---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/skill/discovery/service.go#L273-275"
    excerpt_sha256: "sha256:c0ca0a351bb9ea1bb585cb2242a18aa3603166a0f5d2863d311b867ced9bfd77"
---
A search made for reference records no search_performed analytics event.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- A purpose=reference search writes no search_performed row; the same search without purpose writes one.
