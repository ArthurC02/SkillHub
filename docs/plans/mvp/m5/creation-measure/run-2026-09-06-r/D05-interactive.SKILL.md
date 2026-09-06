---
name: onboarding-flow-to-skill
description: Use this skill when you need to turn an onboarding flowchart or onboarding-process text into a structured, executable step list without adding unsupported steps.
---

# Purpose
Turn the input onboarding flow into a structured, executable step list.

## Instructions
1. Read the input exactly as given.
2. Identify the flow nodes and keep their names in the same order as provided.
3. Convert each node into a clear step in a consistent structure.
4. Preserve the node wording; do not invent extra steps, conditions, roles, dates, or branches.
5. If the input is silent on any detail, write **not given**.
6. Output the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
7. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
8. Walk the flow nodes in order. Do not skip any node.
9. If the input does not provide enough information to complete the artifact, say **not given** for the missing part and continue with the rest of the input.

## Required output shape
- Title the result as a step list.
- List each node in order.
- For each node, include:
  - the node name
  - a brief executable wording based only on that node
  - any missing detail as **not given**
- Keep the output concise and structured.

## Node order to follow
1. 收到新人到職通知
2. 建立公司信箱
3. 加入 Slack 頻道
4. 準備筆電與門禁卡
5. 安排第一週訓練
6. 指派導師
7. 第 30 天面談
8. 記錄到人資系統