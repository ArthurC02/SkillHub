---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-19T17:00:33.921369Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch.go#L278-280"
    excerpt_sha256: "sha256:09a1f6e21cf7b540601c05cee4fb9677bfce3886d0189b068d5717227c89cffb"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_integration_test.go#L332-375"
    excerpt_sha256: "sha256:97a20ef07e639edb269a472b52c57ade107f8b83f7adbc381782462397866e8b"
---
An evaluation batch waits when Run refuses a trial with a refusal Run says will pass on its own, pauses with the reason recorded when the refusal awaits a credit top-up, and stops with the reason recorded on any other refusal.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A credit_balance refusal pauses the batch with only its first trial run; a missing permission confirmation stops it.
