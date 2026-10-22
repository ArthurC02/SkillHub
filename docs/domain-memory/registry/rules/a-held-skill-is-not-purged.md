---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T21:18:54.679348Z"
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
    excerpt_sha256: "sha256:bc2879540fef3ef47d73a4a59cc46f4b2b0f401a1a7afcf156e2ebf4d0f0b019"
  - resource: "apps/platform/internal/skill/library/takedown.go#L16-20"
    excerpt_sha256: "sha256:97744b9d15042311d43d8f1b245577757bcfb2cf7bbcc45b1e56844ab03e3401"
  - resource: "apps/platform/internal/skill/library/restriction.go#L24-28"
    excerpt_sha256: "sha256:ce9617d0879ca7c8567941b98c9798c634760c6f250c2346bc55bd71bb6b41b5"
  - resource: "db/queries/skills.sql#L121-127"
    excerpt_sha256: "sha256:8ab7b9d355a9d498a9aeaf54b90c17f7321176e221bc666b3769a8cc7e2fb754"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/operator_hold_deletion_integration_test.go#L82-130"
    excerpt_sha256: "sha256:6045aad5bf1816fc475abc6f20d25662638a1caa274f36f17ba74a10d23fa4ea"
---
A purge keeps the row of a skill that has test cases or a version referenced by a run, a download or a bundle; the deleted-skill sweep also keeps a skill while its publication is unretired or retired less than the ninety-day name hold ago, and while an operator takedown or access restriction is in force on it. An operator can take down or restrict a skill its owner has deleted until the sweep purges it, and the operator's skill search lists such a skill with when it was deleted.

# Relations
- Contexts: [registry](/contexts/registry)

# Examples
- A deleted skill past grace that has test cases survives the sweep.
- A deleted skill whose publication retired 89 days 23 hours ago survives the sweep; at 90 days and a minute it is purged.
- A skill its owner deleted while an operator takedown was in force survives the sweep past grace; after the operator lifts the takedown the next sweep purges it.
- A skill its owner deleted before any complaint is found by the operator's search, taken down or restricted, and then survives the sweep past grace; a deleted skill nobody holds is purged.
