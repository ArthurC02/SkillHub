---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T06:02:10.436002Z"
sources:
  - resource: "apps/platform/internal/trial/execution/quota.go#L73-80"
    excerpt_sha256: "sha256:7a191da88de1d14f4ae6ccbc8273634be9fbe1270b98e409356490e4afda90eb"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/beta_integration_test.go#L234-251"
    excerpt_sha256: "sha256:4c20906aa085ffe628cd92ebba5768b0173304623cd4362a721efa0729e142d3"
---
A Workspace's run allowance counts only its own runs, so another Workspace's runs never spend it.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- After alice runs three times, bob still has his whole allowance.
