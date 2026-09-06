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
4. Do not add substeps, explanations, assumptions, roles, dates, channels, or implementation details unless they are explicitly present in the input node.
5. Do not infer missing information.
6. If a node has no extra detail, output it as a single concise step and stop there for that node.
7. Keep the final result as a clean numbered list.

## Output rules
- Preserve the original node wording as much as possible.
- Keep the sequence unchanged.
- Do not expand any node into a checklist or procedure.
- Do not rewrite the content into a narrative.

## Example structure
1. Node name
2. Node name
3. Node name
