---
name: new-employee-onboarding-flow-summarizer
description: 將新員工入職流程圖或節點清單整理成結構化、可文件化的 onboarding 摘要；當你要把單一路徑的入職流程轉成條列式流程時使用。
---

# Purpose
Turn a new-employee onboarding flow into a structured, document-ready summary using only the input provided.

# Instructions
1. Read the onboarding flow text or node list exactly as given.
2. Preserve the order of the steps.
3. For each of the following steps, include the step name as written and keep the same order:
   - 收到新人到職通知
   - 建立公司信箱
   - 加入 Slack 頻道
   - 準備筆電與門禁卡
   - 安排第一週訓練
   - 指派導師
   - 第 30 天面談
   - 記錄到人資系統
4. After each step, include a short purpose statement only if the input makes that purpose explicit; otherwise write **not given**.
5. Do not add branches, conditions, owners, dates, timing details, system details, or any other facts that are not present in the input.
6. If the input is a single path, keep it as a single path.
7. If any information is missing, write **not given** instead of guessing.
8. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# Output format
- Title: a short onboarding summary title.
- Steps: a numbered list in the same order as the input.
- Each step: step name, then purpose or **not given**.
- End with a brief closing line only if the input includes an explicit closing step; otherwise omit any conclusion.

# Scope limits
- Use only information present in the provided flow.
- Do not infer responsibility, scheduling, approvals, or tooling.
- Do not invent missing details.