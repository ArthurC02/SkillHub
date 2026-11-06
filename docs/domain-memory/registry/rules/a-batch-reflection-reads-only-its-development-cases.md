---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-06T19:38:45.654816Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection.go#L147-181"
    excerpt_sha256: "sha256:9b652a61f6fe2f34584cd34c377eb6ac326e046d3570c2a04597bf0591090c77"
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection.go#L213-216"
    excerpt_sha256: "sha256:ee3cd79f8c21afac92618f8e8aa392d7538e952f4e1612fd518e8b8121cead39"
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection.go#L197-211"
    excerpt_sha256: "sha256:a7b4357d754bafe6ce889b42a88b6095bc8902de8e4e1c146ae966bb9cc4cb8a"
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection_test.go#L61-92"
    excerpt_sha256: "sha256:c3191b526185f89b357ee6600f53496224918285a98da1771299033750c420b9"
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection_test.go#L36-45"
    excerpt_sha256: "sha256:f6f7b41c2fedb7f9d64c150b839262bae863b129104da1f398a38805cb2f61fa"
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection_test.go#L23-34"
    excerpt_sha256: "sha256:32ec6de817d8017994ce8b759164e4ddd507c53fb6cbf119a41f57d2d0cecfd2"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/reflection_integration_test.go#L184-211"
    excerpt_sha256: "sha256:8d7e6b21d27409a345f1f1b602f6e01cc9203d4b3e8ff860ad2a436e6bc1307b"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/reflection_integration_test.go#L264-301"
    excerpt_sha256: "sha256:514d42bcaef9f15d252c69e5c8181cf88bea311e2571c6f2bf2971f081ee3d90"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/reflection_integration_test.go#L303-329"
    excerpt_sha256: "sha256:bd4b52143d77bffb2d70d927936efe070a656ca712e04bfd292ccfc1aa93d9d6"
---
An improvement round over an evaluation batch reads only the failing runs of the test cases the creator chose for development and holds every other case out for checking the result: it needs at least one development case and at least one held-out case, each named once and from this batch, refuses while the batch is launching or while any development trial is not settled, and reads at most eight failing runs, taking every case's first trial before any second and saying how many it left unread.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A batch of three cases with all three chosen for development is refused, and so is a round naming no case or one case twice.
- A development case with a trial still running is refused until it settles, while a held-out case with no runs yet does not hold the round back.
- Nine failing development runs are read as eight, and the round records one run not read.
