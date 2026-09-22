# Draft PR: All thirteen Contexts say where they live

## Business intent

A coding Agent deciding which Bounded Context owns the work in front of it should be able to ask the Registry and get an answer. Today it gets silence for eleven of the thirteen Contexts, and the only place that answers is a Markdown table scheduled for deletion.

## Domain impact

All thirteen Bounded Contexts gain or replace a Registry record. Ownership decision: a Go package under `apps/platform/internal` belongs to exactly one Bounded Context. Boundary decision: unchanged — this records what each Context is and where it lives; it opens no collaboration and closes none.

## Implementation handoff

Three forces drove it. Eleven Contexts had no record at all. The subdomain, implementation path and requirement prefixes lived only in the table being retired. A responsibility drifts from the code unless it is cited from the package that holds it.

Chosen approach: model all thirteen, citing the machine identity file for subdomain and path and the owning package's own doc comment for responsibility, and carry the requirement prefixes the table would otherwise take with it.

Rejected: leaving the eleven unmodelled until needed (schedules the silence rather than preventing it); keeping the prefixes in the identity file (which requirements a Context owns is domain knowledge, and only thirteen of thirty-one identity rows have any); citing the Context Map table (that is the dependency being removed).

Counterfactual: widening `eval`'s implementation path from `trial/improvement` to `trial` made the validator refuse it as overlapping `run`'s and `testlab`'s.

## Proposal and approvals

`contexts-carry-their-identity`, revision 1, superseding `contexts-cite-their-owners` — the proposal whose approval made the `run` and `registry` records reviewed. Required role: developer. No outstanding decisions.

## Contract impact

None. No API or Event contract changes.

## Verification

Seven obligations, all executed on staged copies of the Registry: structural validity under `--require-reviewed`; every cited excerpt current; no citation into the retired map; two refusal checks (overlapping implementation paths, and a subdomain outside the closed set); the audit chain intact from its recorded head; and an external approval path available.

## Residual risks

The dependency policies between these Contexts are not yet modelled, so `depguard-deny` still reads its whitelist from the document being retired. The document cannot be deleted until that moves.
