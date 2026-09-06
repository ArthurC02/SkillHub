---
name: employee-reimbursement-flow-summary
description: Summarize an employee reimbursement flowchart into nodes, conditions, branches, and uncertainties. Use this skill when you need a structure-preserving reading of a reimbursement process diagram without inventing missing steps or branches.
---

# Instructions

Use this skill when you are given an employee reimbursement flowchart and need a structure-preserving summary of it.

## Non-negotiable rules

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## What to produce

Produce four sections in this order:

1. `nodes`
2. `conditions`
3. `branches`
4. `uncertainties`

## How to work

1. Read the input as a process flowchart summary task.
2. List the process nodes exactly as they appear in the input, without adding or renaming steps.
3. List the decision conditions exactly as they appear in the input.
4. Describe each branch using only the relationships explicitly given in the input.
5. If the input does not state something needed for a section, write `not given`.
6. If there are no uncertainties, state that `uncertainties` is empty.

## Output shape

Return a concise structured result with the four sections above.

### nodes
- List the nodes in flow order.

### conditions
- List each decision condition.

### branches
- For each condition, show the branch outcome that the input states.

### uncertainties
- List only items that cannot be confirmed from the input.
- If none are present, write `empty`.

## Constraints

- Do not infer hidden steps.
- Do not add alternate paths.
- Do not explain your reasoning.
- Do not ask follow-up questions unless the input itself is missing the flowchart content.