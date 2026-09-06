---
name: onboarding-flow-to-skill
description: Use this skill when you need to turn an onboarding flowchart or onboarding-process text into a structured, executable step list without adding unsupported steps.
---

# Purpose
Turn onboarding flow input into a structured step list that preserves the source nodes and order.

## Instructions
1. Read the provided onboarding flow text or diagram content exactly as given.
2. Identify each node in the order it appears.
3. Output one step per node using the node text itself as the step label.
4. Do not add any roles, outputs, substeps, explanations, assumptions, dates, channels, or implementation details unless they appear verbatim in the source input.
5. Use only the wording that appears in the source nodes; if a node is present, output that node and nothing else for that step.
6. Do not infer, expand, or complete missing information.
7. Keep each step to the source node label only, then continue to the next node.

## Output rules
- Preserve the original node wording exactly.
- Do not append any extra details, even when they seem helpful or standard for onboarding.

## Example structure
1. Node name
2. Node name
3. Node name