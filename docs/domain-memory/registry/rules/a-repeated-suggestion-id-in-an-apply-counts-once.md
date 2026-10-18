---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T09:26:27.855880Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L471-476"
    excerpt_sha256: "sha256:4d58676a58d5382ee2a60a538a9892c8f630ccf020c22c5c8b401d99daf83dc5"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_integration_test.go#L299-366"
    excerpt_sha256: "sha256:8f66fb5513b7ce1e8945797c22a28e9f52f91ceb9ac5999ba4fc973c8747856b"
---
An apply request that names the same suggestion twice applies it once.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- suggestion_ids [S, S] builds one version.
