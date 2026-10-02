# Draft PR: the preflight interaction follows its code to the held facts

## Business intent

An Agent asking how Run and Registry collaborate should receive the preflight interaction with a
citation that verifies. Run creation now reads the package scan and the provider answers before
its transaction and hands them to preflight, which moved the cited lines.

## Domain impact

Contexts `registry` (producer) and `run` (consumer). One existing interaction,
`registry-run-preflight-facts`, keeps every field and has its citation rebuilt. The contract
`registry-run-facts-v1` is unchanged.

## Implementation handoff

The struct that carries what run creation already read gained the scan and provider answers, and
the Version read moved out of the summary into its own function beside the public entry. What the
lines establish is the same: the Version is read through the injected Registry reader, and a
missing reader is refused before anything is read. The new citation runs from the public entry to
the end of that function.

Rejected: leaving the read inline, which keeps the summary over its length limit and makes the
citation span types unrelated to Registry; citing only the single call to the reader.

Counterfactual: with the citation left where it was, `verify-evidence` exits 1 and reports it
stale; rebuilt, a staged copy reports 62 current and none stale.

## Proposal and approvals

`run-preflight-facts-follow-the-held-facts`, revision 1, superseding
`run-preflight-facts-follow-their-lines`. One developer approval is required.

## Contract impact

`registry-run-facts-v1` is reused unchanged.

## Verification

Seven obligations, each backed by a digest of the command's own output: `validate` and
`verify-evidence` on a staged copy, `verify-evidence` on the current Registry as the
counterfactual, `analyze-boundary`, the unit test for the refusal, the integration test that run
creation reads nothing while the test case is locked, and `verify-audit`.

## Residual risks

That the reader is the only path to Version facts rests on the cited code and the import guard,
not on a test of its own.
