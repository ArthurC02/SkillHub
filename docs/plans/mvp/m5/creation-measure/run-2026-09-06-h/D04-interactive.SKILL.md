---
name: log-error-escalation-skill
description: Use this skill when you need to turn a confirmed workflow diagram about server log error handling into a portable Agent Skill draft. It preserves the diagram order and records only the steps, branch, and uncertainties already confirmed.
---

# Purpose
Turn a confirmed workflow-diagram understanding into a portable Agent Skill draft.

Use this skill when the input is a confirmed diagram understanding and you need a reusable skill package that follows the diagram exactly, without inventing missing details.

# Inputs
- A confirmed diagram understanding in JSON form.
- The diagram may include nodes, conditions, branches, and uncertainties.
- If the input is missing or incomplete, stop and ask for the missing confirmed information rather than guessing.

# Output
Produce a complete Skill draft with:
- a valid manifest
- a Markdown body
- no extra workflow steps beyond the confirmed diagram
- no invented tools, outputs, or branch logic

# Procedure
Follow the confirmed diagram in the exact order shown below.

## 1. 讀取伺服器 log 檔
Read the server log file as provided by the input.

The diagram does not say where the log comes from or what format it uses, so do not assume either.

## 2. 篩出 ERROR 行
Select the log lines marked ERROR.

The diagram does not say whether any deduplication, aggregation, or other counting rule applies, so state that this is unspecified if you need to mention it.

## 3. 錯誤是否超過 10 筆
Evaluate the condition of whether the number of errors is more than 10.

The diagram only shows this condition; it does not define extra thresholds or alternative conditions.

## 4. 是：超過 10 筆錯誤時，從判斷節點往下執行建立 Jira 問題單 → 通知值班工程師 → 寫入每日摘要
If the confirmed condition is true, perform the following steps in order:
- 建立 Jira 問題單
- 通知值班工程師
- 寫入每日摘要

Do not add any extra action, recipient, field, or notification channel that is not confirmed by the diagram.

## 5. 否：未超過 10 筆錯誤時，從判斷節點直接走到寫入每日摘要
If the confirmed condition is false, go directly to:
- 寫入每日摘要

Do not insert any additional step on this branch.

# Uncertainties
The diagram explicitly leaves these points unspecified, so preserve them as unknown rather than inventing details:
- 讀取伺服器 log 檔的輸入來源或格式
- 篩出 ERROR 行與錯誤是否超過 10 筆之間是否有去重、彙總或其他統計規則
- 建立 Jira 問題單與通知值班工程師的具體內容、對象或通知方式
- 寫入每日摘要的輸出位置、格式或內容範圍

# Writing rules
- Keep the body in Markdown.
- Keep the node order exactly as confirmed in the diagram.
- Do not add steps, conditions, roles, or tools that are not present in the diagram.
- When the diagram is silent, say so plainly instead of filling in details.
- Do not ask the user follow-up questions unless the input itself is missing required confirmed information.

# Packaging notes
- Use the manifest fields exactly as provided in the draft object.
- Keep the skill portable by avoiding environment-specific assumptions.
- Do not claim support for tools unless the skill actually needs them.

# License
Apache-2.0