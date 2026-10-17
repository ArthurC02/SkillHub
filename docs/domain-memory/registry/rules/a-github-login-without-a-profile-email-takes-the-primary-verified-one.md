---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T17:11:22.999935Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/github.go#L124-142"
    excerpt_sha256: "sha256:9674ee19536ccb9a0d165569d198ebbc02ef8432870c20a1998c83561d1aba1c"
  - resource: "apps/platform/internal/creator/workspace/github_test.go#L93-108"
    excerpt_sha256: "sha256:44ba3a921e28f24d919cd95d276af3bc5b34bf94dbcc290adb7d97b5979d6e94"
---
When the GitHub profile carries no email, the login uses only the address GitHub marks both primary and verified.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- Of a verified old address and a primary verified one, the primary is used.
