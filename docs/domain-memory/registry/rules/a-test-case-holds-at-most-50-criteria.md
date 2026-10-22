---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T09:01:27.787074Z"
sources:
  - resource: "apps/platform/internal/trial/design/testlab.go#L640-642"
    excerpt_sha256: "sha256:f5065104f232d1879c455e454199732e37886328586f6043ec3eb55a1c6a2b53"
  - resource: "apps/platform/internal/trial/design/testlab_test.go#L184-203"
    excerpt_sha256: "sha256:475ac1e8de06fea989845dfe405c4b4d36c77c6cb3e5e417af1ae52bf92f3e1d"
  - resource: "apps/platform/internal/trial/design/testlab.go#L43-43"
    excerpt_sha256: "sha256:b6fa3916607a1c82184b7ad47e27619b37e6c5904113cdc8445bba062a111f8c"
---
A Test Case holds at most as many acceptance criteria as the judge reads in one evaluation, twenty; one more is refused as over the limit, so no Test Case can carry a criterion the judge would never see.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- The 20th criterion is accepted and the 21st refused.
