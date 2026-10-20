---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-20T01:20:44.160947Z"
sources:
  - resource: "apps/platform/internal/creator/workspace/purge.go#L174-176"
    excerpt_sha256: "sha256:967aa0a3f90f304861f7160f7ef836055e0dc4a8dd404cdbbb5478d5d5358787"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/account_purge_characterization_test.go#L316-329"
    excerpt_sha256: "sha256:bf7efbfe1710750daf7a843278d804dcfb9426c6616dc7efd56b2ded5a60aca9"
---
Once an account purge has claimed the account, it records the account in object storage, outside the database, before it removes any file or row; when that record cannot be written the purge stops and the account and its files stay as they were.

# Relations
- Contexts: [identity](/contexts/identity)

# Examples
- With the object store refusing writes, a sweep over an account due for purge reports the record failure, removes no object, and leaves the account undeleted.
