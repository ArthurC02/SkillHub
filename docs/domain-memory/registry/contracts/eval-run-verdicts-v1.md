---
type: Contract
status: stable
kind: internal-fact-reader
version: v1
compatibility_policy: "The run list shows each run's current evaluation verdict as eval serves it; run does not interpret it."
data_classification: internal
verified:
  - by: "human:ArthurC"
    at: "2026-10-13T05:56:46.247322Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/verdicts.go#L19-33"
    excerpt_sha256: "sha256:57e0fb6c434b5987b727382051fe6118347651df4a83ca63357e17f80b887105"
  - resource: "apps/platform/internal/trial/improvement/verdicts.go#L78-105"
    excerpt_sha256: "sha256:c1c17840e5de6188d949b944f58d123d74b859f5b09ac951ac5959e4992b82e7"
  - resource: "apps/platform/internal/trial/execution/http.go#L465-482"
    excerpt_sha256: "sha256:562bb58b302958352ef06f8e051bd6972094206bf5c857dc537ba861c1af5544"
---
# Relations
- Producer context: [eval](/contexts/eval)
- Consumer contexts: [run](/contexts/run)
