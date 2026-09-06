---
name: server-log-error-escalation-skill
description: 將伺服器 log 圖解流程整理成可重用的 Agent Skill；當你要把已確認的流程節點、條件、分支與不確定處轉成可執行技能時使用。
---

# Purpose
把使用者提供的流程圖理解轉成一個可重用的 Skill，且只依照已確認的流程、條件、分支與不確定處理。

# When to use
在輸入已經明確描述伺服器 log 流程、ERROR 篩選、門檻判斷與後續動作時使用。若輸入缺少流程內容，先回覆缺少的資訊；不要補寫未提供的步驟。

# Instructions
1. Use only what the input contains — never add a date, name, assumption, input field, summary field, notification channel, step or branch the input does not give. If the input is silent, say 'not given'.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the input as a single flow and process the nodes in this exact order:
   - 讀取伺服器 log 檔
   - 篩出 ERROR 行
   - 判斷錯誤是否超過 10 筆
   - 若錯誤超過 10 筆，則建立 Jira 問題單、通知值班工程師，最後寫入每日摘要
   - 若錯誤未超過 10 筆，則直接寫入每日摘要
4. If the input does not provide a detail for a node, state 'not given' rather than inventing it.
5. Keep the condition exactly as given: 錯誤是否超過 10 筆。
6. Preserve the uncertainty about 每日摘要 by stating that its contents and format are not given.
7. Do not add any step, condition, role, tool, retry path, failure branch, or branch beyond the confirmed flow. Represent only these two branches: if errors exceed 10, create the Jira issue, notify the on-call engineer, then write the daily summary; otherwise, write the daily summary directly.

# Output requirements
Produce a structured result that names the nodes, states the condition, shows both branches, and records the stated uncertainties. Do not turn unknowns into facts.

# Uncertainties
- 是否在兩個分支都先執行同一種錯誤統計方式: not given.
- 每日摘要的具體內容與格式: not given.