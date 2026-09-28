# Draft PR: the finality rule follows the state machine to its new lines

## Business intent

An Agent loading the Run Context should receive the rule that a terminal status is final with
citations that verify. After the state machine lost one import, all three citations pointed one
line too low and the rule read as stale.

## Domain impact

One Context, `run`. One existing Rule, `run-terminal-status-is-final`, keeps its statement and
examples and has its three citations moved. No boundary, contract or ownership changes.

## Implementation handoff

Two helpers in the state machine stopped taking a database transaction directly. That removed
an import and moved every line below it up by one. The three cited excerpts — the transition
table, the terminality test and the transition guard — are byte-identical to what was reviewed:
rebuilding the citations at the new lines reproduces the excerpt digests the rule already carried.

Rejected: padding the file so the old line numbers stay true, and leaving the citations stale
until the rule next changes.

Counterfactual: with the citations left where they were, `verify-evidence` exits 1 and reports
all three stale; moved, a staged copy reports 51 current and none stale.

## Proposal and approvals

`run-terminal-status-follows-its-lines`, revision 1, superseding `run-terminal-status-is-final`,
the proposal that reviewed the rule. One developer approval is required.

## Contract impact

None.

## Verification

Six obligations, each backed by a digest of the command's own output: `validate` and
`verify-evidence` on a staged copy, `verify-evidence` on the current Registry as the
counterfactual, the two unit tests for terminal statuses, `get-context` for `run`, and
`verify-audit`.

## Residual risks

A citation is line-addressed, so the next edit above these lines moves them again.
