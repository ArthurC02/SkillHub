---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T10:57:40.060126Z"
sources:
  - resource: "apps/platform/internal/skill/library/purge.go#L30-32"
    excerpt_sha256: "sha256:c2abe36b9b8910dce5fa36fa4d38ea43b9d698b61470cbf444fbd146ffd2c441"
  - resource: "apps/platform/internal/skill/library/purge_test.go#L45-73"
    excerpt_sha256: "sha256:280894e25a26feb7089be8921c52b7d5ac1d3225efa4834c3e5ba2772f28bcb3"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/publishing_integration_test.go#L719-753"
    excerpt_sha256: "sha256:9dd45542dbe00bd2e4e042590d0e326a07e57a43dcb4d625fc10ab43de51a0e1"
  - resource: "apps/platform/internal/skill/library/purge.go#L126-134"
    excerpt_sha256: "sha256:740c1c6027baaceccc632dbcab93e90d552ae3e31910235701ed49ebdcd4726e"
  - resource: "db/queries/governance.sql#L228-231"
    excerpt_sha256: "sha256:190e8561239a6638065ea9082bdbd505a86f92361e6dc5b5ba2a8f78cbc555e4"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/operator_hold_deletion_integration_test.go#L11-69"
    excerpt_sha256: "sha256:226eea57802bfe6a599a5f215fdc8d268b887baa5a1f0aecd37400f7fb867e4a"
---
A purge keeps the row of a skill that has test cases or a version referenced by a run, a download or a bundle; the deleted-skill sweep also keeps a skill while its publication is unretired or retired less than the ninety-day name hold ago, and while an operator takedown or access restriction is in force on it.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A deleted skill past grace that has test cases survives the sweep.
- A deleted skill whose publication retired 89 days 23 hours ago survives the sweep; at 90 days and a minute it is purged.
- A skill its owner deleted while an operator takedown was in force survives the sweep past grace; after the operator lifts the takedown the next sweep purges it.
