---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/improvement/purge.go#L14-14"
    excerpt_sha256: "sha256:073e7657ce020f26bd57ed87aa4ec1c0f7519347db6e69b57301c829af39ca43"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_integration_test.go#L943-986"
    excerpt_sha256: "sha256:1a511d744463bc6dba5fd2597066850a252bbdaad94d35bcaad8b959c9caa6ad"
---
Purging an account erases the free-text feedback comments on its workspace's evaluations and keeps the evaluations themselves.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- After purge an evaluation that held 'my private note' has no comment and still exists.
