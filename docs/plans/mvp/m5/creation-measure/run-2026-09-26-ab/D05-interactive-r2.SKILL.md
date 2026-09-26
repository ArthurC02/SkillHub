---
name: new-hire-onboarding-flow
description: Creates a reusable Skill that turns a confirmed new-hire onboarding flow diagram into a step-by-step procedure. Use it when the input is a confirmed onboarding flow with a fixed order and you must not invent missing branches or steps.
---

# New Hire Onboarding Flow

Use this Skill when the input is a confirmed new-hire onboarding flow diagram and you need to restate the flow as a reusable step sequence.

## Limitations

只依據已確認的圖示內容撰寫；不補充未畫出的步驟、分支或條件。

## Instructions

1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the confirmed flow in order.
4. Restate each node as the process step.
5. Preserve the exact sequence shown in the confirmed diagram.
6. If the input is silent about a detail, write 'not given' rather than filling in the gap.
7. Do not add branching, looping, parallelism, approvals, or other structure not shown in the input.
8. Output the finished artifact directly.

## Required step sequence

- 收到新入職通知
- 建立公司信箱
- 加入 Slack 頻道
- 準備筆電與門禁卡
- 安排第一週訓練
- 指派導師
- 進行第 30 天面談
- 記錄到人資系統

## Output expectations

- Return the onboarding flow as a direct, usable procedure.
- Keep the wording aligned with the confirmed nodes.
- If the input provides no extra context, do not infer any.