# Draft PR: Run's definition moves to the decision that owns it

## Business intent

The definition of Run should live where its history is kept and where a reader
looking for Run orchestration already goes, and the Registry record should
point at that line.

## Domain impact

One Context, `run`. No boundary is crossed and no ownership changes. The
record's definition is unchanged; only the evidence it rests on moves.

## Implementation handoff

The definition landed in the Context Map for one reason: the gate that keeps
decision numbers out of ordinary prose was also refusing a Registry citation to
a decision document, so the decision that owns Run orchestration was
unreachable as evidence. That gate now exempts Domain Memory's JSON records,
whose citations carry a content digest and go stale the moment the cited line
moves — the failure the gate exists to prevent cannot happen to them. The
Markdown in the same tree, including this file, stays under the rule.

Two reasons to move rather than leave it:

- The Context Map is scheduled for removal once this Plugin matures. A
  definition there would take the record's evidence with it.
- Git already keeps the history of a decision document, which is the property
  the definition needs.

This proposal supersedes `run-term-reviewed-context-map`, the proposal whose
approval made the record reviewed. Naming it is what allows the replacement:
a reviewed record is not overwritten by a proposal that does not say which
approval it replaces.

Counterfactual: staging the replacement with its definition removed made
`validate` report `vocabulary.json:run is reviewed but missing definition` and
exit 1. It ran on a copy; the copy was discarded and the tracked Registry is
unchanged.

## Proposal and approvals

`run-term-reviewed-decision`, revision 1, material, superseding
`run-term-reviewed-context-map`. One developer approval is required, and the
proposer is not eligible to give it.

## Contract impact

None. No API or event contract changes.

## Verification

Six obligations, each backed by a digest of the command's own output.
`validate` and `verify-evidence` ran against a staged copy holding the
replacement record, so the evidence covers the record being proposed rather
than the one still stored: valid, and 6 of 6 citations current including the
decision line. Plus `verify-audit` (valid over 31 events), `resolve-terms`,
`governance-readiness` (ready, no blocks), and the counterfactual above.

The decision document was added to the confirmed source corpus, because a
record may not rest on a file the corpus does not cover.

## Residual risks

The definition is derived from the schema and the state machine; a later change
to what a Run references makes the decision line and this record wrong
together. The signature check that backs the approval is a local pre-push hook,
so a machine without it, or a push with `--no-verify`, does not enforce this
policy.
