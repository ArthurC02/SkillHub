---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T17:11:22.999935Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/http.go#L75-78"
    excerpt_sha256: "sha256:a1e2a72f8e5868b95ddd26407e4bf965c1adcb4194a8aca76d146f22d97ac3df"
  - resource: "apps/platform/internal/creator/workspace/http_test.go#L34-49"
    excerpt_sha256: "sha256:434ad64af8102470abc001f76704971f08b61c83a80582c1fae6c20d47782ab7"
---
A development login name longer than 64 characters is refused with 400 before any account is touched.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A 64-character name logs in; 65 characters answer 400.
