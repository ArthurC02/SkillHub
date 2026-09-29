# Draft PR: connect immutable Versions to their Run evidence

## Business intent

A Skill owner can judge one immutable Version from its complete Run evidence before deciding to rerun, revise, package, or publish it.

## Domain impact

Run Orchestration remains the owner of Run history. The new Version filter only narrows the existing Session-derived Workspace read and adds no cross-Context write, event, consistency boundary, or Registry fact.

## Implementation handoff

Domain facts: Run owns lifecycle and its history is Workspace scoped; Skill Version and historical Run identities are immutable. Forces: pagination makes browser-side filtering incomplete, foreign filters must not widen a read, and execution status must remain separate from evaluation. Decision: no tactical pattern; add one exact optional predicate in the Run owner query and one cache-key dimension in the web client. Unknowns: none. Rejected alternatives: browser filtering, a Version-owned Run endpoint, a copied projection, and a new publish gate. Proof obligations are listed in `test-obligations.json`; mutation removes the Version predicate and must make the focused integration test fail before restoration.

## Proposal and approvals

Proposal `version-run-evidence` is a material additive public-contract change. Developer approval remains outstanding until the repository's configured signed-commit and CI governance records it.

## Contract impact

`GET /runs` gains optional `skill_version_id`. Omission preserves existing behavior; supplying it intersects with `test_case_id`. The response schema and data classification do not change.

## Verification

Planned evidence covers exact filtering before pagination, Workspace isolation, malformed and nonexistent identifiers, combined filters, UI state distinctions, exact continuation links, responsive layout, generated-client drift, and a counterfactual mutation.

## Residual risks

The feature must not ship from a package that lacks required contract approval, and a locally green implementation does not override a red CI workflow.
