---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-22T05:08:48.592136Z"
sources:
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L332-375"
    excerpt_sha256: "sha256:97a20ef07e639edb269a472b52c57ade107f8b83f7adbc381782462397866e8b"
  - resource: "apps/platform/internal/trial/batch/batch.go#L277-282"
    excerpt_sha256: "sha256:9a314bdcfa0e4bf33fe6ed064481f873d11a9c462093eae2253ff570f871a157"
  - resource: "apps/platform/internal/trial/execution/batch_facts.go#L40-51"
    excerpt_sha256: "sha256:3ed0ac9bad6e46ca0157c2b48d8aa1b77a609de28b83605192dc3c59990a5de9"
  - resource: "apps/platform/internal/trial/execution/refusal_passes_test.go#L13-61"
    excerpt_sha256: "sha256:3c3adb560ba58920f1c35fcb618a3a464fa1bf48892c058c37fc65aced100e99"
---
An evaluation batch waits when Run refuses a trial with a refusal Run says will pass on its own, including a run quota that could not be counted; pauses with the reason recorded when the refusal is one the owner can outwait or fix, a credit top-up or a used-up daily or windowed run quota; and stops with the reason recorded on any other refusal.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A credit_balance refusal pauses the batch with only its first trial run; a missing permission confirmation stops it.
- A quota_daily or quota_window refusal pauses the batch; a quota_unavailable refusal makes it wait.
