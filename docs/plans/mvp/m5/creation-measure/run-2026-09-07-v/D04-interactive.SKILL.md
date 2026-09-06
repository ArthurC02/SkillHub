---
name: server-log-error-escalation-skill
description: 將伺服器 log 圖解流程整理成可重用的 Agent Skill；當你要把已確認的流程節點、條件、分支與不確定處轉成可執行技能時使用。
---

# Purpose
把使用者提供的流程圖理解轉成一個可重用的 Skill，且只依照已確認的流程、條件、分支與不確定處理。

# When to use
在輸入已經明確描述伺服器 log 流程、ERROR 篩選、門檻判斷與後續動作時使用。若輸入缺少流程內容，先回覆缺少的資訊；不要補寫未提供的步驟。

# Instructions
1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
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
7. Do not add any step, condition, role, tool, or branch beyond the confirmed flow.
8. If the input is missing the flow itself, stop and ask for that missing input only.

# Output requirements
Produce a structured result that names the nodes, states the condition, shows both branches, and records the stated uncertainties. Do not turn unknowns into facts.

# Flow walkthrough
## 讀取伺服器 log 檔
先取得並讀取伺服器 log 檔；讀不到時只能說 not given，不能補假設。

## 篩出 ERROR 行
從已讀取的 log 中篩出 ERROR 行；篩選方法若未提供，寫 not given。

## 判斷錯誤是否超過 10 筆
以篩出的 ERROR 行判斷是否超過 10 筆。條件只寫成這一句，不延伸出其他門檻或統計方式。

## 若錯誤超過 10 筆，則建立 Jira 問題單、通知值班工程師，最後寫入每日摘要
在此分支中，依確認內容執行建立 Jira 問題單、通知值班工程師，然後寫入每日摘要。Jira 的欄位、通知對象細節與每日摘要內容都 not given。

## 若錯誤未超過 10 筆，則直接寫入每日摘要
在此分支中，直接寫入每日摘要。摘要內容與格式 not given。

# Uncertainties
- 是否在兩個分支都先執行同一種錯誤統計方式：not given。
- 每日摘要的具體內容與格式：not given.
