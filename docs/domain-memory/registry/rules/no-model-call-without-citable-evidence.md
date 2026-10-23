---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-23T00:35:44.284386Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/suggest.go#L66-70"
    excerpt_sha256: "sha256:d908a1782e1d710324c5e442413b0d319087b016c87c385e24051075e5d4ccdc"
  - resource: "apps/platform/internal/trial/improvement/suggest_store_test.go#L26-38"
    excerpt_sha256: "sha256:2276cdfd212d6e93c68a6ff6efca7ce36e4683f9a19b37543c16d93591e4c1c3"
---
When the verdict yields no citable evidence, suggestions are not requested and nothing is paid.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A not_met run whose findings carry no evidence makes no suggester call.
