---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T05:41:41.818005Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L259-267"
    excerpt_sha256: "sha256:e238eec0c0131176e4ec23b7886a601f24f453431a717cc5e2a2f1c04bfc959f"
  - resource: "apps/platform/internal/entrypoint/wiring/evaluation_batches.go#L110-124"
    excerpt_sha256: "sha256:d5fd29b1dd40a5647cfceda7ad758c7e3139ba3313362c4ab3732ba5d9959dbb"
  - resource: "docs/adr/ADR-009-evaluation-verdicts-and-judge-trust.md#L104-104"
    excerpt_sha256: "sha256:1d20da8f4fdd83214a82079afecabba38bed668a1673d0a25d0dcdad2d694efd"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L64-103"
    excerpt_sha256: "sha256:e81314b24b08c5ee37b8d39ec9bd58f93f0d7734f3b01ed16175ca4224756eff"
---
An evaluation batch waits when Run refuses a trial with a refusal Run says will pass on its own, and stops with that reason recorded on any other refusal.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- The workspace is already running as many runs as it may: the batch tries the trial again on its next advance.
- The credit balance is too low: the batch stops and records credit_balance as its reason.
