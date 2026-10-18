---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T15:59:29.446208Z"
  - by: "human:ArthurC"
    at: "2026-10-18T16:53:54.156118Z"
sources:
  - resource: "apps/platform/internal/product/operations/proposals.go#L254-258"
    excerpt_sha256: "sha256:2f0f56161722da877351b1edaea85468509b53996c8dfadec4b2d8eb9cb85fb3"
  - resource: "apps/platform/internal/product/operations/proposals.go#L260-278"
    excerpt_sha256: "sha256:91f7e39c69d2231d1a8fba9a2cc3fe4ac98c7ff54b65daf1330922a7e08da56d"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agent_proposals_integration_test.go#L434-462"
    excerpt_sha256: "sha256:fa04e6e00c16e07f292f77331a4f987ebd670421ad3039a98da77f95949140a4"
---
A proposal still running two hours after it started is failed as abandoned; one inside the lease keeps running.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- A proposal running for 2h01s fails; one at 1h59m59s keeps running.
