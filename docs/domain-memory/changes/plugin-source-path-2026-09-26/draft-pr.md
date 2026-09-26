# Draft PR: a Skill Version records which directory of its stored package is its own root

## Business intent

A Creator imports an Agent Plugin as one archive and expects each Skill inside it to work
like any other Skill. Before this change every Skill of a Plugin was created and then
refused by packaging, scanning, diffing and the improvement loop.

## Domain impact

Contexts: registry (producer) and run (consumer). Registry owns where a Skill Version's
content lives, and that now includes which directory of the stored package is that
Version's root. Run keeps reading Registry-owned facts through the reader the composition
root injects; it still never reaches into Registry storage, and the collaboration stays
synchronous.

## Implementation handoff

An Agent Plugin arrives as one archive holding several Skills, so a package root is no
longer a Skill root. The Version records the directory it was validated from — empty when
the package root already is that root — and the pair of object key and directory travels
as one value so a reader cannot take the key and forget the directory.

Rejected: re-zipping each Skill of a Plugin into its own package (leaves every reader
unchanged but stores content the Creator never supplied, and the Plugin is the unit that
was imported); letting each reader walk for `SKILL.md` (a Plugin holds several, so the walk
has to guess which one this Version is).

Counterfactual: replacing the recorded directory with its base name made the unit test
report the wrong paths; the file was restored and compares byte-identical.

## Proposal and approvals

`registry-run-facts-with-source-path`, superseding `run-registry-reviewed-facts-signed`.
Developer approval recorded, carried by a signed commit, and applied.

## Contract impact

`registry-run-facts-v1` gains the recorded root additively. Its compatibility policy
already allows consumer-specific facts to evolve deliberately, so the version stays `v1`.
A consumer that ignores the field keeps the old behaviour, which is correct only for a
Version whose root is the package root.

## Verification

Six obligations executed: registry validate, the rebuilt citation reproducing this
proposal's digests, verify-audit, analyze-boundary resolving the contract, the unit tests
for the recorded directory, and the integration test that builds a downloadable package
for each Skill of an imported Plugin. The whole platform suite and golangci-lint ran clean.

## Residual risks

The sandbox provider request carries only the object key, so a Run of a Plugin-sourced
Skill installs the whole archive and the agent discovers no Skill. Closing it changes the
provider contract and the runtime image, and is outside this proposal.
