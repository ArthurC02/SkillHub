---
type: Aggregate
status: stable
root: Batch Reflection
verified:
  - by: "human:ArthurC"
    at: "2026-11-06T19:56:27.841987Z"
sources:
  - resource: "db/migrations/0149_batch_reflections.sql#L1-29"
    excerpt_sha256: "sha256:4f4695e40ce56f0cd765e1fc39fc22fce49f8b39ca2d510e93855fac8d2bd64d"
  - resource: "db/migrations/0149_batch_reflections.sql#L50-58"
    excerpt_sha256: "sha256:0192f39e0005784a382c33c56fac57435579fde3cf2317a9cfd09f6bdb43a7e7"
  - resource: "apps/platform/internal/trial/improvement/reflection_store.go#L203-238"
    excerpt_sha256: "sha256:45f600a54d9175ecb9ef4b3e12b195937da20af44f243447abc449dcdffa216c"
---
# Relations
- Context: [eval](/contexts/eval)

# Invariants
- A round is stored once in batch_reflections and never updated or deleted outside an account purge; its outcome, its development test cases and its counts of runs read and not read are decided before it is written.
- A round names at least one development test case and records the skill version of its batch, which the composite foreign key to evaluation_batches keeps equal to the batch version.
- Every suggestion in evaluation_suggestions has exactly one owner, an evaluation or a round, so a round suggestion never appears under a run.
