---
name: github-issue-severity-triage
description: Use this skill when you have GitHub issue text and need each issue classified into one of three severity levels with a one-sentence handling recommendation.
---

# GitHub Issue Severity Triage

## What this skill does
Given plain text describing one or more GitHub issues, classify each issue into one of three severity levels and write one concise handling recommendation for each issue.

Use this skill when you need a quick, structured triage summary for issue lists, bug reports, or pasted GitHub issue content.

## Output contract
For every issue in the input, produce:
- the issue title or a short identifier
- one of three severity levels
- one sentence of handling advice

## Severity levels
Use exactly these three levels:
- **High**: blocks core usage, causes data loss, crashes, security exposure, or makes the feature unusable
- **Medium**: significantly degrades the experience, has a workaround, or affects an important but non-blocking path
- **Low**: minor defects, typos, cosmetic issues, or small usability problems

If the input does not explicitly define a company-specific severity policy, apply the levels above consistently.

## Process
1. Read the full issue text.
2. Separate it into individual issues using titles, headings, numbering, or other clear boundaries.
3. For each issue, identify the impact, scope, and urgency from the text.
4. Assign the most fitting severity level using the definitions above.
5. Write exactly one short, actionable recommendation in one sentence.
6. Present the results in a clear table or bullet list that keeps each issue paired with its severity and recommendation.

## Writing rules
- Keep each recommendation to one sentence.
- Do not invent missing facts.
- Base the severity only on the information present in the issue text.
- If the issue text is ambiguous, choose the most defensible severity from the available evidence and say so briefly in the recommendation.
- Preserve the meaning of the original issue rather than rewriting it into a new requirement.

## Preferred response format
Use this structure:

| Issue | Severity | Recommendation |
| --- | --- | --- |
| ... | High/Medium/Low | One sentence ... |

If a table is not practical because the input is unstructured, use bullets with the same three fields for each issue.

## Examples of severity use
- A crash that prevents login is **High**.
- A dark mode contrast problem that leaves the page usable is usually **Medium**.
- A typo in FAQ text is **Low**.

## Constraints
- Do not search the web.
- Do not ask follow-up questions unless the input is missing the issue text entirely.
- Do not output extra analysis beyond the severity label and the one-sentence recommendation for each issue.
- Do not rely on external tooling unless the environment explicitly provides it; this skill is designed to work from the pasted issue text alone.