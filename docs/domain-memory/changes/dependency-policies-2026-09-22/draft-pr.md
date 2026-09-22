# Draft PR: The import whitelist becomes reviewed domain knowledge

## Business intent

Which Bounded Context may call which is a domain decision. It lived in a Markdown table that any edit could widen, in a document now being retired. Move it to the reviewed Registry and let the machine check read it there.

## Domain impact

All thirteen Bounded Contexts. Ownership decision: every user-data query resolves its Workspace scope through `identity`. Boundary decision unchanged in substance — the same collaborations stay permitted and the same ones stay forbidden. What changes is where the permission is recorded and what it takes to add one.

## Implementation handoff

Three forces. The permission list was a document, not a reviewed record. The table granted every Context access to `identity` through a row with a blank left-hand side, and a blanket grant is a hole in the rule that unlisted means denied. Recording the same permission in two places would drift, so the check must read the one place rather than reconcile two.

Chosen approach: one reviewed dependency policy per permitted collaboration, the blanket grant expanded into one policy per Context, and the machine check reading them.

Rejected: keeping the whitelist in the document and citing it (the document is being deleted); recording the forbidden collaborations too (unlisted already means denied); keeping the blanket grant as one sourceless record (the schema requires two named Contexts, and for good reason).

Counterfactual: renaming one policy's `from_context` to a Context the Registry does not hold made the validator refuse it.

## Proposal and approvals

`dependency-policies-enter-the-registry`, revision 1. Required role: developer. No outstanding decisions.

## Contract impact

None.

## Verification

Seven obligations. The one that matters most is mechanical: the twenty-eight staged policies are exactly the ordered pairs the retired whitelist permits, with its blanket grant on `identity` expanded — so this move changes what is allowed by nothing at all.

## Residual risks

Opening a new cross-context call now costs a signed Change Package rather than a table edit. That friction is the point, but it is new, and it falls on whoever next needs a collaboration that does not exist yet.
