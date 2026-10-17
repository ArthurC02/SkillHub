---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T06:47:28.319050Z"
sources:
  - resource: "apps/platform/internal/trial/design/testlab.go#L640-642"
    excerpt_sha256: "sha256:f5065104f232d1879c455e454199732e37886328586f6043ec3eb55a1c6a2b53"
  - resource: "apps/platform/internal/trial/design/testlab_test.go#L184-202"
    excerpt_sha256: "sha256:1b5bb2329d2bd01be3ca06ba94c975cf0e2a636727b9d18059947bda29bd737e"
---
A Test Case holds at most MaxCriteria acceptance criteria; one more is refused as over the limit.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- The 50th criterion is accepted and the 51st refused.
