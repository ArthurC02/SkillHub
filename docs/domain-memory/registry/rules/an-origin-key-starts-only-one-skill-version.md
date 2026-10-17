---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/execution/service.go#L874-891"
    excerpt_sha256: "sha256:b33d744e5dc035dd07b22002b8fd5e71cba0dd4fb652efe02fe1d92a6a91ae62"
---
A caller's origin key returns the run it already started for the same Skill Version and is refused for a different version.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- The same key on another version is refused; on the same version it returns the same run.
