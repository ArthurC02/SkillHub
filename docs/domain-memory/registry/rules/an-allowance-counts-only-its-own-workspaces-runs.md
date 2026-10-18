---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/execution/quota.go#L73-80"
    excerpt_sha256: "sha256:7a191da88de1d14f4ae6ccbc8273634be9fbe1270b98e409356490e4afda90eb"
---
A Workspace's run allowance counts only its own runs, so another Workspace's runs never spend it.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- After alice runs three times, bob still has his whole allowance.
