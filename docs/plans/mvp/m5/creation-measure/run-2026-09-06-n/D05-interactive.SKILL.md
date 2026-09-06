---
name: newhire-onboarding-flow-summary
description: 將新人到職流程圖或單一路徑到職步驟整理成結構化摘要時使用，適合需要保留原始順序並標示圖中未說明處的情境。
---

# Purpose
Use this skill to turn a new-hire onboarding flow into a structured summary while preserving the original order and explicitly keeping the diagram's stated uncertainties.

# Instructions
- Read the input as an onboarding flow description or step list.
- Identify and retain the steps exactly as given in the input.
- Keep the steps in the same order as the input.
- When the input includes these onboarding steps, include them explicitly in the output body in the same order and with the same wording: 收到新人到職通知, 建立公司信箱, 加入 Slack 頻道, 準備筆電與門禁卡, 安排第一週訓練, 指派導師, 第 30 天面談, 記錄到人資系統.
- If the input includes an uncertainty or a note that something is not specified, preserve it explicitly as written or summarize it as "not given" when the input is silent.
- Do not invent decision points, exception paths, or extra branches.
- Keep timing labels such as "第 30 天面談" unchanged.
- Produce the finished onboarding summary directly.

# Output shape
Return a concise structured summary with:
1. A step list in order.
2. A separate note section for any uncertainty or unspecified detail.

# Quality check
Before finalizing, verify that every output item comes from the input and that no new branch, condition, or assumption has been introduced.