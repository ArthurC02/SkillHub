---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-24T10:35:00.265291Z"
sources:
  - resource: "apps/platform/internal/trial/execution/service.go#L894-911"
    excerpt_sha256: "sha256:b33d744e5dc035dd07b22002b8fd5e71cba0dd4fb652efe02fe1d92a6a91ae62"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L169-193"
    excerpt_sha256: "sha256:2b5d23941a92f5b3931f8bb94133ea6bf177ef3edf90c279cb5d34307813dcf8"
---
A caller's origin key returns the run it already started for the same Skill Version and is refused for a different version.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- The same key on another version is refused; on the same version it returns the same run.
