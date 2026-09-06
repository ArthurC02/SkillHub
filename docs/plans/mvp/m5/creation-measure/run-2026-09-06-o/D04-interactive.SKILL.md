---
name: flowchart-to-structured-process
description: Turn a user-provided flowchart into a structured process explanation with ordered steps, conditions, branches, and uncertainties. Use this when the user gives a diagram or flowchart and wants a faithful text version without invented nodes or paths.
---

# Purpose
Convert the flowchart in the user's input into a structured process explanation that preserves the diagram's order, conditions, branches, and any stated uncertainties.

# Instructions
Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

1. Read the user's input exactly as given.
2. Identify the diagram's nodes, conditions, branches, and uncertainties from the input only.
3. Walk the diagram's nodes in order, one by one, and describe each node in the same order the diagram gives.
4. For each condition, state the condition and then state each branch exactly as the diagram shows it.
5. List every node by its exact diagram name, including intermediate steps and terminal steps.
6. Do not add any step, role, condition, branch, or uncertainty that the diagram does not show.
7. If the input is silent about a node detail, branch detail, condition detail, or uncertainty detail, write 'not given'.
8. If the input contains no uncertainty, state that uncertainty is not given.
9. Produce the finished structured process explanation directly in the output.
10. If the user's input is missing the diagram or flowchart content entirely, ask only for that missing content.

# Output shape
Return a concise structured explanation with sections for:
- Nodes, in order
- Conditions
- Branches
- Uncertainties

Keep the output faithful to the input and do not summarize away any node or branch that is present.