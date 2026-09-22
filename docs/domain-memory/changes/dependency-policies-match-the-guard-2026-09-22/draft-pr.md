# Draft PR: The recorded policies agree with the guard

## Business intent

A reader should be able to ask the Registry what a Context may call and get the same answer the compiler enforces. Two records said otherwise.

## Domain impact

`credit` and `policy` are recorded as forbidden from reaching `identity`, which is what the compiled guard always enforced. The other twenty-six are unchanged.

## Implementation handoff

The retired whitelist granted every Context access to `identity` through a row with a blank left-hand side, and the previous proposal expanded that row literally. The guard never agreed: it forbids `credit` and `policy` from reaching `identity`, and the whitelist's own prose says `credit` takes those facts by injection. The previous proposal's check compared the expansion with itself, so it could not see the disagreement; the guard saw it on the first run.

Chosen approach: record those two as forbidden with the reason each is forbidden, and check the whole set against the compiled guard rather than against the expansion it came from.

Rejected: withdrawing the two records (an approved change package only upserts, so a reviewed record is replaced in place and never removed); renaming them to read as prohibitions (a new id leaves the over-permissive record behind, still reviewed and still permitting); loosening the guard to match the records (the guard was right).

Counterfactual: renaming one policy's `to_context` to a Context the Registry does not hold made the validator refuse it.

## Proposal and approvals

`dependency-policies-match-the-guard`, revision 1, superseding `dependency-policies-enter-the-registry`. Required role: developer.

## Contract impact

None.

## Verification

The obligation that matters is computed from the compiled guard's own deny lists, independently of the records: across every guarded Context, each ordered pair is either denied there or permitted here — never both, never neither.

## Residual risks

Two records keep ids that read as permissions while recording a prohibition, because a reviewed record can only be replaced in place. The prose rule that `creation` stays isolated from the reference, admission and trial Contexts is still only in the document being retired; it has no home yet.
