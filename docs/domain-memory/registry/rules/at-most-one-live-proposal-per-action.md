---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T15:14:38.912433Z"
sources:
  - resource: "apps/platform/internal/product/operations/proposals.go#L91-97"
    excerpt_sha256: "sha256:89994fa5c32ede52796e9d47871dc32d689fab8ca202951e16cb11618db2d526"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_agent_proposals_integration_test.go#L126-149"
    excerpt_sha256: "sha256:170cfb346bb7da5da662b9c6233527d92a54076a2a40b22fefc2c52b5631440b"
---
A new proposal is skipped while another proposal for the same action is proposed, approved or running.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- Two daily reports proposing run-purge-audit leave one proposal until the first closes.
