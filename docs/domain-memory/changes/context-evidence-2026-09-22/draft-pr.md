# Draft PR: the Contexts stop resting on a document being retired

## Business intent

The Context Map is scheduled for removal once this Plugin can carry what it
holds. Both reviewed Context records were evidenced by rows of it, so the
Registry was depending on the thing it is meant to replace.

## Domain impact

Two Contexts, `run` and `registry`. No collaboration, contract or invariant
changes. Each record's evidence moves, and each responsibility is restated in
the words of the package that owns it.

## Implementation handoff

A Context record makes two claims: what the boundary is called, and what it is
responsible for. Each now cites the source that actually carries that claim.

- The name comes from the decision that draws the boundaries, which names both
  Contexts on one line along with the aggregate roots inside them.
- The responsibility comes from the doc comment of the package that holds it.
  `trial/execution` says it owns a Skill trial Run's lifecycle — its state
  machine, dispatch to a sandbox provider, following execution to completion,
  and cleanup. `skill/library` says it owns Skills and their immutable
  versions — identity, lineage, fork, delete, takedown, and the licensing and
  redistribution gates on a Skill's materials.

Both responsibilities are now recorded as those packages state them rather
than as the one-line summaries the Context Map rows supported, so each record
says as much as its evidence does. `registry` is also renamed from "Skill
Registry" to "Skill Registry & Versioning", which is the name the boundary
decision gives it.

Three alternatives were rejected: copying the responsibilities into a decision
document, when the package doc comment is closer to the code and already says
it; keeping the Context Map citation alongside the new one, when that
dependency is the thing being removed; and waiting until the Context Map is
actually deleted, which schedules a broken Registry instead of preventing one.

Counterfactual: staging both replacements with `responsibility` removed made
`validate` report `contexts.json:run is reviewed but missing responsibility`
and exit 1. It ran on a copy; the copy was discarded and the tracked Registry
is unchanged.

## Proposal and approvals

`contexts-cite-their-owners`, revision 1, material, superseding
`run-registry-reviewed-facts-signed`, the proposal whose approval made both
records reviewed. One developer approval is required, and the proposer is not
eligible to give it.

## Contract impact

None. No API or event contract changes.

## Verification

Six obligations, each backed by a digest of the command's own output.
`validate` and `verify-evidence` ran against a staged copy holding both
replacement records: valid, 8 of 8 citations current, and none of those eight
pointing into the Context Map. Plus `verify-audit` (valid over 34 events),
`governance-readiness` (ready, no blocks), and the counterfactual above.

The boundary decision joined the confirmed source corpus; the two package doc
comments were already in it.

## Residual risks

`aggregates/run` still cites the DDD convergence handbook. That document is
not scheduled for removal, but it is a handbook rather than a decision.

Two of the Context Map's thirteen Core and Supporting Contexts are modelled.
Deleting that document today would still lose the other eleven rows, so the
Registry is not yet a replacement for it — only independent of it.
