---
type: Business Rule
status: draft
owner: run-execution
version: unknown
effective_from: unknown
effective_until: unknown
sources:
  - resource: "apps/platform/internal/trial/execution/quota.go#L18-26"
    excerpt_sha256: "sha256:470051c4203e4f15d916f3e8a79a2f7d8090ead7abfa7a7140dfac908255d62c"
---
A Run counts against the Workspace's quota only from preparing onward, and not when it failed with provider_error, platform_error, capability_mismatch, policy_refused or resource_limit.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A Run that failed with resource_limit because its sandbox ran out of memory does not use up the Workspace's quota.
- A Run that failed because its own Skill errored counts, as does every Run that reached preparing and succeeded.
