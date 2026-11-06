# Draft PR: Find improvements from an evaluation batch's development cases

## Business intent

A skill creator who ran an evaluation batch picks the development test cases; the model reads only their failing runs and proposes suggestions, and the creator checks the new version on the cases the model never saw.

## Domain impact

Owner: eval. New aggregate Batch Reflection (immutable round, its suggestions owned by it alone). New Interaction batch-eval-batch-trials: batch provides the batch skill, version, status and trial settlement through a composition-root fact reader; eval imports nothing from batch. ingest receives the improved version with reflection_id in its existing improved_by provenance.

## Implementation handoff

The orchestration and HTTP live in the reflection subpackage of eval; eval exports package functions to read a run failure, store a round and list rounds, and the Reflection aggregate reuses the suggestion ledger for decide and apply. batch_reflections stores skill_version_id under a composite foreign key to evaluation_batches so apply needs no batch reader. Counterfactuals: accepting a development set that names every case fails the decision-table test; skipping the billing admission fails the below-threshold test.

## Proposal and approvals

batch-reflection-recited-2026-11-07, superseding batch-reflection-2026-11-07 whose three test citations moved before review. Developer approval by ArthurC.

## Contract impact

public.yaml adds reflectOnEvaluationBatch, listEvaluationBatchReflections and BatchReflection; from-suggestions takes reflection_id as the alternative owner. Additive. domain-events.md adds the batch_reflection aggregate and the optional reflection_id on improved_by; events written before decode unchanged.

## Verification

Seven obligations, unit and integration, recorded with record-test-result; both rules have a killed counterfactual. Six reviewed rules are re-cited where the suggestion ledger and apply path moved.

## Residual risks

No round has run on a real model. Forcing every evaluated run to count as worth reading is not caught by a test: no fixture reaches a met run with only info findings without a sandbox trace.
