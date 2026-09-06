---
name: new-hire-onboarding-flow-summary
description: 整理新進人員到職流程圖的節點與不確定處；當輸入是流程圖、流程文字或同等描述，且需要輸出結構化整理結果時使用。
---

# Purpose
Turn the input into a structured summary of the onboarding flow without adding anything that is not explicitly present.

# Confirmed nodes to include
- 收到新入到職通知
- 建立公司信箱
- 加入 Slack 頻道
- 準備筆電與門禁卡
- 安排第一週訓練
- 指派導師
- 第 30 天面談
- 記錄到人資系統

# Rules
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give.
- Keep the node names exactly as confirmed above.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# Procedure
1. Read the input exactly as given.
2. List every confirmed node in the order provided by the input.
3. State any conditions or branches only if the input gives them.
3a. 必須輸出「分支條件：沒有明顯分支條件。」以及「起訖節點原文可能需要確認：『收到新入到職通知』與『記錄到人資系統』。」兩句，不得省略。
4. Record any uncertainties that are present in the input or confirmed understanding.
5. Output the finished structured result directly.

# Output shape
Provide a concise structured result with these sections, in this order:
- Nodes
- 分支條件：沒有明顯分支條件。
- Conditions
- Branches
- Uncertainties

If a section has no content from the input, write 'not given'.

# Uncertainties
- 起訖節點原文可能需要確認：『收到新入到職通知』與『記錄到人資系統』。