---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/purge.go#L276-296"
    excerpt_sha256: "sha256:c4b66529e4c8621f1fd079b3d377ca3c6eb2786f42c93bc3b4e125ddb0371cea"
  - resource: "apps/platform/internal/creator/workspace/purge.go#L126-155"
    excerpt_sha256: "sha256:d609244eb9cceb505a10fb8056ca18f16be8d842492b5f869f1d7fad77781efc"
  - resource: "apps/platform/internal/creator/workspace/purge_grace_integration_test.go#L15-46"
    excerpt_sha256: "sha256:ce16089656ce432c4662d7f0e9c79995d1a4a7bd0e66e3a4808e35e21dcd706e"
---
An account purge starts only when the deletion request is older than the 30-day grace at the moment of claiming and every Run in each of the Account's workspaces has finished cleanup; otherwise the purge is deferred (not failed) and retried by a later sweep.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- A request made now is not claimed by a sweep whose cutoff is 30 days ago; the Account does not start purging.
- An Account with a Run still cleaning up its sandbox is skipped this sweep without an error.
