---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T09:26:27.855880Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/apply.go#L486-488"
    excerpt_sha256: "sha256:9bbf91046f9f69865287efad08cd2c4941a369faacf5fd8e1ab7f2178e95d2a3"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/suggestion_integration_test.go#L299-366"
    excerpt_sha256: "sha256:8f66fb5513b7ce1e8945797c22a28e9f52f91ceb9ac5999ba4fc973c8747856b"
---
Applying a suggestion that is not accepted is refused, naming it, and no version is built.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- Applying a pending suggestion answers 400.
