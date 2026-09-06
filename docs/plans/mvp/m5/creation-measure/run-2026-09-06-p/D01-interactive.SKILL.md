---
name: employee-expense-flow-summarizer
description: Summarize an employee expense flowchart into a clear text workflow. Use this when the input is a single flowchart and you need a faithful step-by-step summary of nodes, conditions, and branches without adding unstated rules.
---

# Employee Expense Flowchart Summarizer

Turn the flowchart in the input into a clear text workflow summary.

## Instructions

1. Read the flowchart exactly as given.
2. Preserve the node names exactly as they appear in the diagram.
3. Preserve the condition names exactly as they appear in the diagram.
4. Preserve the branch labels and arrow order exactly as they appear in the diagram.
5. Include every node and decision shown in the flowchart.
6. Do not add any step, rule, branch, or interpretation that is not explicitly present in the flowchart.
7. When you summarize, keep the output faithful to the diagram’s wording and sequence.
8. Deliver the finished summary itself; do not explain the method or ask for more input.

## Output shape

Write a concise workflow summary that includes:
- the start node,
- each decision point,
- each branch outcome,
- each downstream action node in order.

## Example of faithful behavior

If the diagram shows:
- `收到員工報帳申請`
- `發票是否齊全`
- `退回並要求 3 天內補件`
- `送經理簽核`
- `財務直接入帳`
- `寄出付款通知信`

then the summary must use those same node names instead of paraphrases.

## If something is missing

If a node, condition, or branch is not shown in the flowchart, omit it rather than inventing it.