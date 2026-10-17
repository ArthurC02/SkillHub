---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:14:38.912433Z"
sources:
  - resource: "apps/platform/internal/product/operations/actions.go#L28-30"
    excerpt_sha256: "sha256:a3147c40b2fe0faca25d252f03bac9247052baac6dd9a5805d43442684ee7150"
  - resource: "apps/platform/internal/product/operations/proposals.go#L73-75"
    excerpt_sha256: "sha256:f7e1c5f2cb945950843500e54fc9088c19c5045fb6f447d8e93cdde803ec72e6"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agent_proposals_integration_test.go#L151-171"
    excerpt_sha256: "sha256:df2f303a5a7be3927ae8fa9fabbb62fb0921c38b10681855fdb82e2a80a4236d"
---
A proposal whose preview counts are all zero is dropped and never reaches an operator.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A purge-feedback preview of zero feedback rows records no proposal.
