---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T16:04:44.915924Z"
sources:
  - resource: "apps/platform/internal/skill/delivery/testcase.go#L126-131"
    excerpt_sha256: "sha256:388f56d03a152c3a1270d5413761fc5abb4eee8f9acbeea0a3511fcd7caa6d37"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/packaging_integration_test.go#L983-1008"
    excerpt_sha256: "sha256:77e55701aa91c0bf8236fe122f847cc1f299f033117179756ef7be444e7c468c"
---
When the user opts out of test cases, even curated cases stay out and are listed as user_opted_out.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A catalogue Skill packaged without test cases lists its curated case as user_opted_out.
