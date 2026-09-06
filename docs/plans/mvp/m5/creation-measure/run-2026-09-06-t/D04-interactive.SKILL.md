---
name: flowchart-understanding-skill
description: Turn a provided flowchart into a structured understanding with nodes, conditions, branches, and uncertainties. Use this when the user gives a flowchart image or text and wants a faithful, non-invented summary.
---

# Purpose
Turn the flowchart the user provides into a structured understanding that lists nodes, conditions, branches, and uncertainties.

# When to use this skill
Use this skill when the user provides a flowchart image or flowchart text and wants a faithful structural summary rather than a redesign, interpretation, or implementation.

# Instructions
1. Read the flowchart exactly as provided.
2. Walk the diagram in order, following each node and branch that is shown.
3. Output the structure as:
   - nodes
   - conditions
   - branches
   - uncertainties
4. Keep the wording faithful to the diagram.
5. If the diagram is silent about a detail, write "not given".
6. Do not add steps, conditions, roles, tool use, or branches that are not shown.
7. If the input is incomplete, stop and report only what the input contains.
8. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
9. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

# Required walk-through for this flowchart
1. 讀取伺服器 log 檔
2. 篩出 ERROR 行
3. 錯誤是否超過 10 筆
4. 當錯誤超過 10 筆時，進入建立 Jira 問題單、通知值班工程師、寫入每日摘要
5. 當錯誤未超過 10 筆時，直接進入寫入每日摘要

# Output shape
Provide the result as a structured list or JSON with these fields:
- nodes
- conditions
- branches
- uncertainties

# Notes
- The only confirmed condition is whether the error count exceeds 10.
- The branch labels are 是 / 否.
- Where the flowchart does not explain how error counts are computed, write not given.
- Where the flowchart does not explain the Jira issue contents or on-call notification contents, write not given.