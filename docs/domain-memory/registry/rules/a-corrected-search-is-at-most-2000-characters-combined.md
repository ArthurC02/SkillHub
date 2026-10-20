---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-20T14:46:47.256731Z"
sources:
  - resource: "apps/platform/internal/skill/discovery/intent.go#L100-102"
    excerpt_sha256: "sha256:20a3b297e2c0a30297180e30aacafa117d16b184e51e8c746d74d04070e626f0"
  - resource: "apps/platform/internal/skill/discovery/intent_test.go#L105-118"
    excerpt_sha256: "sha256:ab43daa8acd23224bc5c8156eee0a8911e08dbd734b65934705eab1a23e814f0"
---
A search the person corrected is refused when its intent fields and keywords together exceed 2000 characters; exactly 2000 is accepted.

# Relations
- Contexts: [catalog](/contexts/catalog)

# Examples
- A corrected intent totalling 2000 characters searches; 2001 is refused.
