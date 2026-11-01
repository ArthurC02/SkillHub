# Draft PR: a check on a cut final output cannot fail on what was cut

## Business intent

A skill author reading an evaluation is never told the answer missed a value, or stayed within a length limit, on the strength of a part the runtime cut off. Such a check now says it cannot tell and names where the output was cut.

## Domain impact

eval gains the rule a-check-on-a-cut-final-output-cannot-fail-on-what-was-cut. The rule a-pass-is-never-recorded-on-incomplete-evidence is unchanged. No other Context or collaboration changes.

## Implementation handoff

The trace summary reads the truncated flag the runtime already writes on the final agent_output event and serves it as final_output_truncated. Material.FinalOutputCut gives the reason with the kept character count. contains_values records undetermined for a missing value on a cut answer; the length limits record undetermined for a within-limit result on a cut answer; the judge lists final_output as cut for a runtime cut. Rejected: showing the cut only on the page, and treating a cut as making all evidence incomplete. Proof: ignoring the flag in FinalOutputCut fails the cut-answer rows, then restored.

## Proposal and approvals

cut-final-output-2026-11-02, one developer approval; ruled by the Optimization session on the user's instruction to settle design questions by best practice.

## Contract impact

TraceSummary gains the optional boolean final_output_truncated.

## Verification

rule-a-check-on-a-cut-final-output-cannot-fail-on-what-was-cut maps to the rule check table and TestACutAnswerSaysWhereItWasCut; judge-runtime-cut maps to TestAFinalOutputTheRuntimeCutIsSentAsCut.

## Residual risks

A judge fail on a cut answer still stands; the judge is told the answer was cut and may answer undetermined itself.
