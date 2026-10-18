---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:52:30.207485Z"
sources:
  - resource: "apps/platform/internal/skill/library/skill_store.go#L32-34"
    excerpt_sha256: "sha256:d5fd3029310c94182bbde8dd2c99c36eec9ff21c1494cfe57782348af43aa5ac"
  - resource: "apps/platform/internal/skill/library/content_guards_test.go#L20-44"
    excerpt_sha256: "sha256:f0e0bd3e111b39090b3831e370fd1ff97adc4a3062ec27511bc2d1a83a1353f3"
---
A taken-down skill's package is never reported as current content.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- After a takedown, the current-package check for its newest version answers false.
