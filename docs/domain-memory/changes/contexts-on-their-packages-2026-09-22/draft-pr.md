# Draft PR: A Context rests only on the package that holds it

## Business intent

Adding a Go package should not make the machine checks go red. It did: the thirteen Context records cited the architecture identity file, which put that file in the Registry's source corpus, and source verification compares whole-file snapshots. A one-byte edit was enough.

## Domain impact

All thirteen Bounded Contexts. Ownership decision unchanged: a Go package under `apps/platform/internal` belongs to exactly one Bounded Context. Boundary decision unchanged — this narrows what each record rests on and alters no collaboration.

## Implementation handoff

A tool whose job is to confirm facts must not make routine work fail. The directory a package occupies is already proved by the path of the doc comment being cited, so a second citation into a layout file added a dependency without adding evidence.

Chosen approach: cite only the owning package's doc comment, and let the machine checks read the Registry directly rather than treating layout data as a source to corroborate against.

Rejected: keeping the layout citation and re-confirming the corpus each time (that is the cost being removed, paid forever); dropping `implementation_path` from the records (the machine checks are about to read it from here, so it is the one place it should live); keeping both the Registry and the layout file authoritative for the thirteen Contexts (one fact with two homes drifts).

Counterfactual: widening `eval`'s implementation path to `trial` made the validator refuse it as overlapping `run`'s and `testlab`'s.

## Proposal and approvals

`contexts-rest-on-their-packages`, revision 1, superseding `contexts-carry-their-identity`. Required role: developer. No outstanding decisions.

## Contract impact

None.

## Verification

Seven obligations. Alongside structural validity, current citations, an intact audit chain and an available external approval path, two are specific to this change: no record names the layout file, and appending a byte to that file leaves source verification reporting current — the failure this change exists to remove.

## Residual risks

The cross-context dependency whitelist still lives in the document being retired, so that document cannot be deleted yet.
