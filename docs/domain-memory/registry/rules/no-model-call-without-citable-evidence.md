---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T09:26:27.855880Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/suggest.go#L47-53"
    excerpt_sha256: "sha256:d465ec824cbe401b649e2a8d1661ca968bb9ee7b79f3e9f5bac6e82c34d337cc"
  - resource: "apps/platform/internal/trial/improvement/suggest_store_test.go#L26-38"
    excerpt_sha256: "sha256:2276cdfd212d6e93c68a6ff6efca7ce36e4683f9a19b37543c16d93591e4c1c3"
---
When the verdict yields no citable evidence, suggestions are not requested and nothing is paid.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A not_met run whose findings carry no evidence makes no suggester call.
