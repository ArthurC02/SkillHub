# Draft PR: Run gets a definition

## Business intent

A coding Agent reading Domain Memory should be able to ask what a Run is and
receive an answer. Run is this system's central term, used in every
architecture decision, specification, handbook and package comment, and
defined in none of them.

## Domain impact

One Context, `run`. No boundary is crossed: the term is defined inside the
Context that owns it. Ownership is unchanged — Run owns lifecycle transitions,
and that invariant already lives on the Run aggregate.

## Implementation handoff

The previous record defined Run as "Skill trial lifecycle." and cited a line of
the DDD convergence table that enumerates the commands the Run aggregate
accepts. That line does not carry the definition, which is why the record was
withheld when the surrounding Run and Registry facts were promoted.

An exhaustive read of the architecture decisions, the goal and specification
plans, the six handbooks under `docs/development/`, the `trial` package doc
comments and the public OpenAPI contract found no line anywhere that defines
the term. The definition was therefore derived from what the schema and the
state machine establish, and written into the Context Map, the document that
already says what each Bounded Context is.

Three readings were rejected against evidence rather than taste:

- "One execution of a Skill" — `run_attempts` is unique on
  `(run_id, attempt_number)`, so an attempt is the execution and the Run is the
  retryable unit whose identity a reassignment does not change.
- "…whose result is evaluated" — `running` transitions directly to `failed`,
  `cancelled` or `timed_out`, so not every Run reaches `evaluating`.
- Defining it in the architecture decision that owns Run orchestration — this
  repository reserves decision numbers and filenames for those documents and
  their index, so a Registry citation to one is refused by the gate that
  enforces it.

Counterfactual: marking the term reviewed with its definition removed made
`validate` report `vocabulary.json:run is reviewed but missing definition` and
exit 1. It ran on a copy; the copy was discarded and the tracked Registry is
unchanged.

## Proposal and approvals

`run-term-reviewed-context-map`, revision 1, material, superseding
`run-term-reviewed`. One developer approval is required, and the proposer is
not eligible to give it.

## Contract impact

None. No API or event contract changes.

## Verification

Six obligations, one per acceptance criterion plus the invariants behind them,
each backed by a digest of the command's own output: `validate`,
`verify-evidence` (6 of 6 citations current), `verify-audit` (valid over 26
events), `resolve-terms` (the term resolves from a whole-sentence query),
`governance-readiness` (ready, no blocks), and the counterfactual above.

Editing the Context Map changed the source snapshot the corpus was selected
under, so the sources were refreshed and reconfirmed. The two records already
citing that file stayed current: a citation is verified line by line, not
against the whole file.

## Residual risks

The definition is derived from the schema and the state machine and recorded in
the Context Map; a later change to what a Run references makes that line and
this record wrong together. The signature check that backs the approval is a
local pre-push hook, so a machine without it, or a push with `--no-verify`,
does not enforce this policy.
