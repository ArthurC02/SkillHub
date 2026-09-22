# Draft PR: Creation reaches its collaborators through inversion

## Business intent

Two things the retired Context Map carried are things a compiler cannot: why interactive Skill creation stays isolated, and why two Contexts are kept away from `identity`. Both now live in the reviewed Registry, and no record reads as the opposite of what it says.

## Domain impact

`creation` gains one reviewed rule: everything it needs from the reference, admission and trial Contexts arrives as an injected interface, and no Context imports it. The two policies that recorded a prohibition under an id reading `may-use` are removed and re-recorded as `credit-must-not-use-identity` and `policy-must-not-use-identity`. Nothing the guard enforces changes.

## Implementation handoff

The guard already denies every pair this covers, because unlisted means denied. What it cannot carry is the reason, and the reason is one reason, not twenty-two forbidden rows. Recording the matrix would restate the default and bury the invariant that matters: the creation loop must be replaceable without any other Context changing.

The two mis-named policies existed because, until now, an approved change could only write a record — a reviewed one could be overwritten in place but never withdrawn. The plugin now applies `operation: "remove"`, so the ids can be retired instead of inherited.

Chosen approach: state the inversion once as a rule on `creation`, and replace the two policies by removing them and writing the correctly named prohibitions in the same approved change.

Rejected: recording all twenty-two forbidden pairs; leaving the reason in a handbook outside the reviewed model; leaving the two ids alone.

Counterfactual: stripping the new rule's statement made the validator refuse it and exit 1.

## Proposal and approvals

`creation-reaches-out-through-inversion`, revision 1, superseding `dependency-policies-match-the-guard`. Required role: developer.

## Contract impact

None.

## Verification

Seven obligations, all executed on a staged copy of the Registry holding the change. The one that constrains the result is computed from the compiled guard's own deny lists: across every guarded Context each ordered pair is either denied there or permitted here, never both and never neither. The rest cover structural validity, citation currency, the audit chain, the external approval path, and a scan proving no policy id contradicts its policy field.

## Residual risks

None outstanding from the retired document. Everything it stated that the guard does not enforce is now a reviewed record.
