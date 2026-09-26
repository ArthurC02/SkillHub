---
name: flowchart-understanding-skill
description: Interpret an already confirmed flowchart into a concise, ordered process summary and drafting-ready Skill requirements. Use this when the user has provided or confirmed a flowchart and wants the result without adding any unstated steps or assumptions.
---

# Flowchart understanding

Use this Skill when the user provides a flowchart or a previously confirmed interpretation of one and wants a faithful, concise understanding of the process.

## Core rules
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Procedure
1. Read the input exactly as given.
2. Follow the flowchart nodes in order, and name each step exactly as the diagram names it.
3. For each decision point, state the condition exactly as shown.
4. For each branch, state only the actions shown on that branch.
5. If the diagram is silent about a detail, write 'not given' instead of inventing it.
6. Do not add extra steps, conditions, roles, tools, outputs, or exception handling that the diagram does not show.
7. If the input does not include a flowchart or a confirmed diagram understanding, say that the needed input is not given.

## Output
Return a direct, concise result that contains:
- the ordered nodes,
- the decision condition(s),
- the branch outcomes,
- and any explicit uncertainties that remain.

Keep the result faithful to the diagram and do not explain the rules you used.