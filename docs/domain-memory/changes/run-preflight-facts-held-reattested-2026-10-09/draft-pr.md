# Draft PR: the preflight interaction is attested by a commit that carries it

## Business intent

An Agent asking how Run and Registry collaborate should receive the preflight interaction backed
by a review that covered it. The earlier package recorded this interaction, but its attested
commit carries a different package, so nothing shows that the signed review covered this proposal.

## Domain impact

Contexts `registry` (producer) and `run` (consumer). The existing interaction
`registry-run-preflight-facts` is recorded again unchanged, from the current Registry record. The
contract `registry-run-facts-v1` is unchanged.

## Implementation handoff

No code changes. The record, its citation and its contract stay as they are. Only the review is
redone: the package is recorded again, approved again, and attested by a signed commit that
carries this proposal.

Rejected: editing the old package's attestation in place, which would rewrite an applied review
record; leaving it, which keeps `audit-attestations` reporting it as not carried.

## Proposal and approvals

`run-preflight-facts-held-reattested`, revision 1, superseding
`run-preflight-facts-follow-the-held-facts`. One developer approval is required.

## Contract impact

`registry-run-facts-v1` is reused unchanged.

## Verification

Six obligations, each backed by a digest of the command's own output: `validate` and
`verify-evidence` on the Registry, `analyze-boundary`, the unit test for the refusal, the
integration test that run creation reads nothing while the test case is locked, and
`verify-audit`.

Counterfactual: with the refusal disabled, the earlier form of the refusal test still passed,
because a missing test lab refused first. The test now injects a test lab and requires the
Registry refusal itself; under the same mutation it fails.

## Residual risks

That the reader is the only path to Version facts rests on the cited code and the import guard,
not on a test of its own.
