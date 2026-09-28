# Draft PR: the preflight interaction follows its code to the reshaped lines

## Business intent

An Agent asking how Run and Registry collaborate should receive the preflight interaction with a
citation that verifies. After preflight's parameters were grouped into one named target, the cited
lines changed and the interaction read as stale.

## Domain impact

Contexts `registry` (producer) and `run` (consumer). One existing interaction,
`registry-run-preflight-facts`, keeps every field and has its citation rebuilt. The contract
`registry-run-facts-v1` is unchanged.

## Implementation handoff

The preflight functions took the Skill, Version and Test Case ids as three parameters and now
take them as one value. What the lines establish is the same: the Version is read through the
injected Registry reader, and a missing reader is refused before anything is read. The new
citation starts at the function's own first line rather than inside its parameter list.

Rejected: keeping the separate parameters to spare the citation, and citing only the single call
to the reader.

Counterfactual: with the citation left where it was, `verify-evidence` exits 1 and reports it
stale; rebuilt, a staged copy reports 51 current and none stale.

## Proposal and approvals

`run-preflight-facts-follow-their-lines`, revision 1, superseding
`run-registry-reviewed-facts-signed`, the proposal that reviewed the interaction. One developer
approval is required.

## Contract impact

`registry-run-facts-v1` is reused unchanged.

## Verification

Six obligations, each backed by a digest of the command's own output: `validate` and
`verify-evidence` on a staged copy, `verify-evidence` on the current Registry as the
counterfactual, `analyze-boundary`, the unit test for the refusal, and `verify-audit`.

## Residual risks

That the reader is the only path to Version facts rests on the cited code and the import guard,
not on a test of its own.
