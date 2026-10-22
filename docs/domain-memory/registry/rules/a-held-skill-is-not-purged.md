---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T04:51:07.826389Z"
sources:
  - resource: "apps/platform/internal/skill/library/purge.go#L30-32"
    excerpt_sha256: "sha256:c2abe36b9b8910dce5fa36fa4d38ea43b9d698b61470cbf444fbd146ffd2c441"
  - resource: "apps/platform/internal/skill/library/purge_test.go#L45-73"
    excerpt_sha256: "sha256:280894e25a26feb7089be8921c52b7d5ac1d3225efa4834c3e5ba2772f28bcb3"
  - resource: "apps/platform/internal/skill/library/purge.go#L110-114"
    excerpt_sha256: "sha256:e9f818746e1f8791396134115a4a8e838d0021652ae3c55f20eaa13f15f51a19"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/publishing_integration_test.go#L719-753"
    excerpt_sha256: "sha256:9dd45542dbe00bd2e4e042590d0e326a07e57a43dcb4d625fc10ab43de51a0e1"
---
A purge keeps the row of a skill that has test cases or a version referenced by a run, a download or a bundle; the deleted-skill sweep also keeps a skill while its publication is unretired or retired less than the ninety-day name hold ago.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A deleted skill past grace that has test cases survives the sweep.
- A deleted skill whose publication retired 89 days 23 hours ago survives the sweep; at 90 days and a minute it is purged.
