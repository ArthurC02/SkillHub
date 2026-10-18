---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T06:26:10.741232Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/http.go#L558-565"
    excerpt_sha256: "sha256:940b4f8e0b04d8d4849edb0a08f14efdf10499e5d17e5d8738104e5aaa9041e6"
  - resource: "apps/platform/internal/skill/discovery/http_test.go#L11-18"
    excerpt_sha256: "sha256:555961ce00cd66c6987e0b1bf02cd110e77d435a48997ed0fbd0484c904e4dd5"
---
A search query longer than 2000 characters, counted in runes, is refused; exactly 2000 is accepted.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- 2000 Chinese characters are searched; 2001 answer 400.
