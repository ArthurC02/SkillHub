# Code expression

Reviewed facts constrain what the code must do. This page is about whether the next reader can find those facts in the code. It carries no numeric threshold: line counts, complexity limits, and parameter limits belong to the repository's own automation, where they are enforced.

## Before writing

1. **Find the repository's gates.** Run `quality-gates --repo-root <repo>` and read the configuration it lists for the command that runs each check. Done means those are green. When the repository enforces none, say so in the report. Do not present a number or a named style guide as the project's standard, and do not add a tool or a configuration nobody asked for.
2. **Look for what already exists.** Search for a helper, type, or constant that already does part of the work, and use it. A rule written a second time is a rule that will be changed in one place.
3. **Read the neighbours.** Follow the local idiom, including the repository's convention on comments.

## Names

- A term in the reviewed vocabulary is the identifier. A synonym is a second term the next reader has to reconcile.
- A number or string that is a business fact gets the domain's name for it and one home.
- An extracted function takes the name of the rule it decides. When the only name available describes mechanics, the extraction has not found a rule.

## Shape

Extract when a rule has a name, when nesting hides which conditions lead to an outcome, or when one rule is written twice. Do not extract a function that only forwards to another, and do not split a decision so that following it means opening several functions. A few functions that each settle something read better than many that each pass something along.

Values that always travel together are a concept; name it. A flag that selects between two behaviours is two operations. State a refusal before the work it prevents. Add no layer, option, or extension point for a case the requirement does not contain.

## Preserving behaviour

A refactoring claims that nothing observable changed. Prove that claim:

1. List the rules the code decides and find the test for each. For a rule with no test, write one against the unchanged code, through the public entry point, before changing anything.
2. Record the names of the tests that exist.
3. Make the change.
4. Compare: every recorded test is still present and passes, and none was skipped.
5. For each rule that was moved or extracted, run `counterfactual` on the line that states it. `killed` is the proof. `survived` is a rule with no test: write the test, then run it again.

A comparison run once and thrown away proves today's change and protects nothing afterwards. Whatever showed that behaviour was preserved stays in the repository as a test, and so does the test behind each proof obligation in the handoff.

Report the rules that had no test before the change. That is a finding about the code as it was, and the next Agent needs it.

## Evidence

| Claim | Evidence |
| --- | --- |
| The change meets the project's standard | The repository's own checks ran and passed, or the report states that it has none. |
| Nothing was written twice | A search for the rule finds one home. |
| Behaviour is preserved | The recorded tests are present and pass, and each moved rule failed its test when broken. |
| An abstraction is earned | The requirement contains the second case, or something external stands behind it. |

## Leave alone

A security check, an output other systems read byte for byte, and a published artifact are not made more readable in passing; changing them is a change of behaviour with its own review. A rewrite whose motive is speed is not a readability change either.

## Proportion

A mechanical change inside a reviewed boundary needs the change, the tests, and a short report. It needs no design record and no handoff.
