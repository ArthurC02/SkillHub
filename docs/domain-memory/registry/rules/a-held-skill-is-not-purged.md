---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T10:52:30.207485Z"
sources:
  - resource: "apps/platform/internal/skill/library/purge.go#L30-32"
    excerpt_sha256: "sha256:c2abe36b9b8910dce5fa36fa4d38ea43b9d698b61470cbf444fbd146ffd2c441"
  - resource: "apps/platform/internal/skill/library/purge.go#L60-88"
    excerpt_sha256: "sha256:a2a7347ffc485b3861736cfb247141709c40470dbf73bbec6e3b177238bace13"
  - resource: "apps/platform/internal/skill/library/purge_test.go#L23-51"
    excerpt_sha256: "sha256:280894e25a26feb7089be8921c52b7d5ac1d3225efa4834c3e5ba2772f28bcb3"
---
A purge keeps a skill that has test cases or a version referenced by a run, a download or a bundle.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A deleted skill past grace that has test cases survives the sweep.
