---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T04:35:41.095408Z"
sources:
  - resource: "apps/platform/internal/skill/admission/service.go#L606-609"
    excerpt_sha256: "sha256:34decd15f5b58566224bf707fbbf6115a1bb4d5cdc28e5ca5e2ccdfec508f446"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/version_refusal_integration_test.go#L13-46"
    excerpt_sha256: "sha256:476af42ae0ea563712e09a36a5580ce7f88be2534f6cd6dbcfcc717b1b953543"
---
A new version must declare the same name as its Skill; another name is refused as version_name_mismatch and neither a version nor the summary changes.

# Relations
- Contexts: [ingest](/contexts/ingest)

# Examples
- Uploading a package named split-csv as a version of tidy-notes is blocked.
