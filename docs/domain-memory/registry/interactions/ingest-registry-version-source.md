---
type: Interaction
status: stable
consistency: synchronous
delivery: admission hands the recorded source id to the new version in the same transaction
verified:
  - by: "human:ArthurC"
    at: "2026-10-23T07:12:38.614001Z"
sources:
  - resource: "apps/platform/internal/skill/admission/service.go#L499-501"
    excerpt_sha256: "sha256:35c4b3b20980d35f1e806531c7a8f51580f0d8026ac4aae82befd5138f7c1535"
---
# Relations
- Producer context: [ingest](/contexts/ingest)
- Consumer context: [registry](/contexts/registry)
