---
type: Contract
status: stable
kind: internal-port
version: v1
compatibility_policy: "Run starts and continues trials for an evaluation batch through the ports the composition root fills; Run publishes whether a refusal passes on its own and never hands its transaction to the batch."
data_classification: internal
idempotency:
  key: "origin key: the batch, the Test Case, the trial number and whether the Skill is withheld"
  scope: per Workspace
  on_replay: "returns the Run already started under that key, without admitting or charging again"
  on_conflict: refused when the key already started a Run of a different Skill Version
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T05:08:48.592136Z"
sources:
  - resource: "apps/platform/internal/entrypoint/wiring/evaluation_batches.go#L46-88"
    excerpt_sha256: "sha256:81f47701455768a93e0f9330943c273785dd34ee7ef9af66468493c2ee38f32d"
  - resource: "apps/platform/internal/trial/execution/service.go#L874-891"
    excerpt_sha256: "sha256:b33d744e5dc035dd07b22002b8fd5e71cba0dd4fb652efe02fe1d92a6a91ae62"
  - resource: "docs/adr/ADR-009-evaluation-verdicts-and-judge-trust.md#L104-104"
    excerpt_sha256: "sha256:1d20da8f4fdd83214a82079afecabba38bed668a1673d0a25d0dcdad2d694efd"
  - resource: "apps/platform/internal/trial/execution/batch_facts.go#L35-51"
    excerpt_sha256: "sha256:4ce1a570d590d6e8dab11f9e8bdebe20db4f3394d49f79ad0ab4207be2f421a7"
---
# Relations
- Producer context: [run](/contexts/run)
- Consumer contexts: [batch](/contexts/batch)
