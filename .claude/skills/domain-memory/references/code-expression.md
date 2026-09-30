# Code expression

Reviewed facts constrain what the code must do. This page is about whether the next reader can find those facts in the code. It carries no numeric threshold: line counts, complexity limits, and parameter limits belong to the repository's own automation.

## Before writing

1. **Find the repository's gates.** Run `quality-gates --repo-root <repo>` and read the configuration it lists for the command that runs each check. A change is not done until those are green, and a check turned green by a change of form alone is not met (see [When a gate forces the change](#when-a-gate-forces-the-change)). When the repository enforces none, say so in the report. Do not present a number or a named style guide as the project's standard, and do not add a tool or a configuration nobody asked for.
2. **Look for what already exists.** Search for a helper, type, query, or constant that already does part of the work, and use it or extend it. A rule written twice gets changed in one place.
3. **Read the neighbours.** Follow the local idiom, including the repository's convention on comments.

## Names

- A term in the reviewed vocabulary is the identifier. A synonym is a second term the next reader has to reconcile.
- A number or string that is a business fact gets the domain's name for it and one home, and every place it appears uses that name, messages included. One concept keeps one name across the repository.
- A name says something the value does not. A constant named after its own value or after the field it fills adds a jump and no meaning.
- An extracted function takes the name of the rule it decides. When the only name available describes mechanics or position, the extraction has not found a rule.
- When a name changes meaning, rename it. A name that still reads correctly but now means something else is worse than an unclear one.

## Shape

Extract when a rule has a name, when nesting hides which conditions lead to an outcome, or when one rule is written twice. Do not extract a function that only forwards to another, and do not split a decision so that following it means opening several functions. A few functions that each settle something read better than many that each pass something along.

An extracted step returns what it decided; it does not report by changing an argument. It hands a failure back to its caller, and only the entry point decides whether the process ends. One piece of state has one owner: when a loop is split, the bookkeeping moves with the rule that needs it.

Values that always travel together are a concept; name it from the reviewed terms and read its fields where they are used. A flag that selects between two behaviours is two operations. State a refusal before the work it prevents. Add no layer, option, or extension point for a case the requirement does not contain, and put nothing in product code that only a test calls. When an existing function needs less than its parameter asks for, narrow the parameter instead of building a stand-in to satisfy it.

## When a gate forces the change

A check that fails on length, complexity, argument count, a flag argument, a literal, a repeated string, or interface size names a symptom, not a remedy. It says where the code is hard to read; the reviewed domain says how to fix it. A change that turns the check green and leaves the reader the same work has moved the cost, not removed it.

Remove the cause, trying these in order:

1. Remove repetition. A shared function for a shape written several times shortens and simplifies more than any split.
2. Extract a rule under the name of the rule it decides.
3. Name values that travel together as the concept they are, and use its fields directly.
4. Replace a flag with the two operations it selects, each named for what it does.
5. Name a value only when the name says what the value does not, and replace every occurrence.

Each of these moves meets a check in form only, and leaves a tell you can search for in your own change:

| Move | Tell |
| --- | --- |
| Bundling arguments into an object that has no meaning of its own | The function's first statement copies the fields back into locals |
| Wrapping a flag in a new type, or in an enum that is only ever asked yes or no | The value is turned back into a boolean, or compared with one member, where it is read |
| Naming a constant after its value or its field | The name contains the number, or the literal still appears in a message or a case label |
| Splitting a loop so two functions share its state | A helper returns nothing and changes an argument, or caller and helper both update the same field |
| Splitting so a step can end the process | A function below the entry point exits, and its signature does not say so |
| Splitting an interface to meet a size limit | No caller depends on one part alone |
| Splitting near-duplicate blocks into named functions | Two places now hold identical function bodies |
| Grouping a long table or setup into named parts | The name of a part needs "and" |
| Copying a query or mapper to vary one filter | Two definitions differ only in a condition |

When no move removes the cause, the check does not fit this code. Say so: record the exception the repository permits, with its reason, or report the conflict to whoever owns the check. An exception is visible; a disguise is not.

## Preserving behaviour

A refactoring claims that nothing observable changed. Prove that claim:

1. List the rules the code decides and find the test for each. For a rule with no test, write one against the unchanged code, through the public entry point, before changing anything.
2. Record the names of the tests that exist.
3. Make the change.
4. Compare: every recorded test is still present and passes, and none was skipped.
5. For each rule that was moved or extracted, run `counterfactual` on the line that states it. `killed` is the proof. `survived` is a rule with no test: write the test, then run it again.

The tests that showed behaviour was preserved stay in the repository, and so does the test behind each proof obligation in the handoff. Report the rules that had no test before the change: that is a finding about the code as it was, and the next Agent needs it.

## Evidence

| Claim | Evidence |
| --- | --- |
| The change meets the project's standard | The repository's own checks ran and passed, and a search of the change finds none of the tells above. Where the repository has no checks, the report says so. |
| Nothing was written twice | A search for the rule finds one home. |
| Behaviour is preserved | The recorded tests are present and pass, and each moved rule failed its test when broken. |
| An abstraction is earned | The requirement contains the second case, or something external stands behind it. |

## Leave alone

A security check, an output other systems read byte for byte, and a published artifact are not made more readable in passing; changing them is a change of behaviour with its own review. A rewrite whose motive is speed is not a readability change either.

## Proportion

A mechanical change inside a reviewed boundary needs the change, the tests, and a short report. It needs no design record and no handoff.
