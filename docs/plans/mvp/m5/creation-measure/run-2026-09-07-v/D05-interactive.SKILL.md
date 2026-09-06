---
name: new-hire-onboarding-flow
description: 將新人到職流程整理成可重複使用的 Agent Skill；當輸入是單一路徑的新人體驗／到職流程時使用，輸出結構化的流程 Skill 草稿。
---

# 目的
將使用者提供的新人到職流程，整理成一個可重複使用的 Agent Skill。

# 何時使用
當輸入是一份新人到職流程的文字或圖像內容，且流程是一條單一路徑、沒有分支時使用。

# 必須遵守的規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 工作方式
1. 讀取輸入中的新人到職流程。
2. 只使用輸入已提供的節點與順序。
3. 依照流程順序整理成結構化內容。
4. 若輸入對某項資訊保持沉默，明確寫成 `not given`。
5. 直接輸出成品，不要輸出分析、規則說明或待確認問題。

# 需要處理的流程節點
依序處理下列節點，且不得新增未提供的節點：
- 收到新人到職通知
- 建立公司信箱
- 加入 Slack 頻道
- 準備筆電與門禁卡
- 安排第一週訓練
- 指派導師
- 第 30 天面談
- 記錄到人資系統

# 輸出要求
- 產出一份可直接重用的 Agent Skill。
- 內容必須明確說明用途、輸入、輸出與限制。
- 不得加入分支條件、例外情況或額外步驟。
- 若某些細節未提供，寫成 `not given`。

# 執行原則
- 保持單一路徑，照既有順序完成。
- 不推測流程之外的背景資訊。
- 不把未提供的資訊補成合理假設。