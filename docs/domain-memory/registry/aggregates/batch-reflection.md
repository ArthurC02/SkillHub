---
type: Aggregate
status: stable
root: Batch Reflection
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T11:52:39.625583Z"
sources:
  - resource: "db/migrations/0149_batch_reflections.sql#L1-29"
    excerpt_sha256: "sha256:4f4695e40ce56f0cd765e1fc39fc22fce49f8b39ca2d510e93855fac8d2bd64d"
  - resource: "db/migrations/0149_batch_reflections.sql#L50-58"
    excerpt_sha256: "sha256:0192f39e0005784a382c33c56fac57435579fde3cf2317a9cfd09f6bdb43a7e7"
  - resource: "db/migrations/0150_reflection_candidates.sql#L23-46"
    excerpt_sha256: "sha256:946ae8a6f18a6ed43a9921c4b8818bd04defc181424f1fac461741dc80ae4c25"
  - resource: "apps/platform/internal/trial/improvement/reflection_store.go#L227-270"
    excerpt_sha256: "sha256:684de5f180f181d4ce8cf6f5900357f76d237e91a21d987c181a64ae4bcf8d10"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/reflection_integration_test.go#L566-589"
    excerpt_sha256: "sha256:666213e4bfa3ff97d19e3da7c3419a708cb7e533405de1de115145cb00cd1382"
---
# Relations
- Context: [eval](/contexts/eval)

# Invariants
- A round is stored once in batch_reflections and never updated or deleted outside an account purge; its outcome, its development test cases and its counts of runs read and not read are decided before it is written.
- A round names at least one development test case and records the skill version of its batch, which the composite foreign key to evaluation_batches keeps equal to the batch version.
- Every suggestion in evaluation_suggestions has exactly one owner, an evaluation or a round, so a round suggestion never appears under a run.
- A round of several candidates records each candidate's focus and outcome in candidate order, in two arrays of equal length, at most four, with no focus repeated; a round of one candidate records both arrays empty, and its outcome is the round's.
- With several candidates the round's outcome is the best of its candidates': kept, then none_kept, then none_proposed, then unavailable.
- Every round suggestion records the index of the candidate that wrote it, from 0 to 3; a suggestion on one run's evaluation records 0.
