# Draft PR: Record the Batch Reflection aggregate

## Business intent

A reader of the schema finds the rule behind each batch_reflections constraint.

## Domain impact

eval gains the reviewed Batch Reflection aggregate. No new collaboration and no code change.

## Implementation handoff

Citations of 0149's constraints and of RecordReflection. Proof obligations one-owner and kept-off-the-run.

## Proposal and approvals

batch-reflection-aggregate-2026-11-07. Developer approval by ArthurC.

## Contract impact

None.

## Verification

Two integration obligations recorded with record-test-result; the immutability SQL test proves the trigger and the composite foreign key.

## Residual risks

None beyond batch-reflection-recited-2026-11-07.
