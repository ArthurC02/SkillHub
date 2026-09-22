# Draft PR: a terminal Run outcome is final

## Business intent

An Agent about to write code that changes a Run's status should learn from the
Registry that a terminal outcome cannot be left, before it writes the path that
would try.

## Domain impact

One Context, `run`. A new Rule record. No boundary, contract or ownership
changes, and no existing record is replaced.

## Implementation handoff

The Run Context has held no Rule since `run-lifecycle-owner` was retracted. That
record said "Run owns lifecycle transitions", which is an ownership statement:
no business scenario violates it. Finality is the constraint that was missing,
and it is falsifiable in the way a Rule asset is for — a late cancel, a late
provider callback and a retry each describe a scenario that would break it.

The constraint is expressed structurally rather than as a check. The transition
table gives successors for `queued`, `provisioning`, `preparing`, `running` and
`evaluating` and gives none for `succeeded`, `failed`, `cancelled` or
`timed_out`. Terminality is then read off the same table — a status is terminal
when the table has no entry for it — and the transition guard refuses anything
the table does not allow. All three are cited, because the rule is only true as
long as they stay one mechanism.

Two alternatives were rejected. Putting it on the Run aggregate's invariants
would pile a testable business constraint onto the record that already carries
ownership. Stating it as a list of terminal statuses would duplicate the table,
so a status added to the table later would leave the rule silently wrong.

Counterfactual: staging the rule with its statement removed made `validate`
report `rules.json:run-terminal-status-is-final is reviewed but missing
statement` and exit 1. It ran on a copy; the copy was discarded and the tracked
Registry is unchanged.

## Proposal and approvals

`run-terminal-status-is-final`, revision 1, material. It replaces no reviewed
record, so it supersedes nothing. `rule_ids` stays empty: it lists the existing
rules a change affects, and this rule does not exist until the change applies.
One developer approval is required, and the proposer is not eligible to give it.

## Contract impact

None. No API or event contract changes.

## Verification

Six obligations — one per acceptance criterion and one per invariant behind
them — each backed by a digest of the command's own output. `validate` and
`verify-evidence` ran against a staged copy holding the rule: valid, and 11 of
11 citations current including all three excerpts of the state machine. Plus
`get-context` for `run`, which is the shape a reader actually receives,
`verify-audit` (valid over 39 events), `governance-readiness` (ready, no
blocks), and the counterfactual above.

The state machine joined the confirmed source corpus.

## Residual risks

The rule is evidenced by the transition table rather than by a test that drives
a Run out of a terminal status. The Registry records the constraint; it does not
execute it.

Two of the Context Map's thirteen Core and Supporting Contexts are modelled.
