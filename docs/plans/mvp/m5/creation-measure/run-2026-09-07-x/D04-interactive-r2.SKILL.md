---
name: server-log-error-escalation
description: 將伺服器 log 中的 ERROR 行計數並依錯誤數量決定是否建立 Jira 問題單、通知值班工程師與寫入每日摘要；在需要把這段流程整理成可重複使用的 Agent Skill 時使用。
---

# 目的
把使用者提供的伺服器 log 流程整理成可重複使用的 Agent Skill，並且只依已確認的節點與分支撰寫。

# 執行原則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access。

# 步驟
1. 讀取伺服器 log 檔。
2. 篩出 ERROR 行。
3. 判斷錯誤是否超過 10 筆。
4. 如果錯誤超過 10 筆，建立 Jira 問題單、通知值班工程師、寫入每日摘要。
5. 如果錯誤沒有超過 10 筆，直接寫入每日摘要。

# 分支
- 是：建立 Jira 問題單 → 通知值班工程師 → 寫入每日摘要
- 否：直接寫入每日摘要

# 輸出要求
- 產出一個可直接重複使用的 Skill 說明。
- 只包含已確認的步驟、條件與分支。
- 若資訊未提供，寫成 'not given'。

# 限制
- 不新增未確認的節點。
- 不新增未確認的工具需求。
- 不推測 log 格式、Jira 欄位、通知方式或摘要格式。