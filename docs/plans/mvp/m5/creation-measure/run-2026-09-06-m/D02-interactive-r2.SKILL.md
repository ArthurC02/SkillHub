---
name: order-flowchart-to-skill
description: Turn a confirmed flowchart into a portable Skill draft that walks the diagram nodes in order. Use this when you need a Skill that must preserve only the steps, conditions, branches, and uncertainties explicitly shown in the diagram.
---

# Purpose
Create a Skill from the confirmed diagram understanding by following the diagram exactly and preserving only what it shows.

# Instructions
Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output schema
When the task is to summarize the confirmed diagram understanding, output exactly four Markdown sections in this order: nodes, conditions, branches, uncertainties.
In `nodes`, list only the directly identifiable node names from the confirmed diagram, one per line, with no added explanation.
In `conditions`, `branches`, and `uncertainties`, include only items explicitly shown in the confirmed diagram; if a section has none, output `[]` for that section.

## Process
1. Read the input you are handed.
2. Identify the diagram nodes and walk them in order.
3. For each node, do exactly what that node says and no more.
4. If the diagram has a condition, branch, or uncertainty, handle it only as named in the confirmed diagram understanding.
5. If the diagram is silent about any detail, write 'not given' rather than inventing it.
6. Output the finished artifact directly.

## Required behavior
- Follow the confirmed nodes in order: 收到客戶 Excel 訂單 → 轉成 CSV 格式 → 檢查缺漏欄位 → 匯入 Shopify 後台 → 回覆客戶已入單.
- Do not add steps, conditions, roles, tools, or branches that are not present in the confirmed diagram understanding.
- If a detail needed for execution is missing from the input, state 'not given'.
- If the input is insufficient to complete the artifact, ask only for that missing input and nothing else.

## Output
Produce the finished artifact itself, not a plan or explanation.
