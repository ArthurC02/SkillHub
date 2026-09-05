---
name: new-hire-onboarding-checklist
description: Create and validate a structured onboarding checklist from a new hire notice or onboarding task data. Use this skill when you need a step-by-step onboarding sequence that stays faithful to the confirmed process diagram and does not add unconfirmed branches or conditions.
---

# New Hire Onboarding Checklist

## Purpose
Turn a new hire notice or onboarding task data into a structured, ordered onboarding checklist that follows the confirmed process exactly.

## When to use this skill
Use this skill when someone provides a new employee onboarding request, arrival notice, or a list of onboarding tasks and needs the steps converted into a clear checklist or execution plan.

## Confirmed process
Follow these steps in order and do not add extra branches, conditions, or steps:
1. 收到新人到職通知
2. 建立公司信箱
3. 加入 Slack 頻道
4. 準備筆電與門禁卡
5. 安排第一週訓練
6. 指派導師
7. 第 30 天面談
8. 記錄到人資系統

## Instructions
1. Read the input and identify the onboarding request details, such as employee name, department, start date, and requested tasks.
2. Produce a checklist or table that lists the confirmed steps in the exact order above.
3. For each step, indicate whether the input explicitly mentions it, and if appropriate, note the supporting detail from the input.
4. If the input is missing information needed to confirm a step, say what is missing instead of inventing details.
5. Do not introduce optional paths, alternative flows, approvals, dependencies, or additional tasks that are not in the confirmed process.
6. Keep the output concise and operational: the goal is to help someone carry out or review onboarding, not to narrate the process.
7. If the user asks for a richer output, still anchor every item to the confirmed eight-step sequence.

## Output format
Prefer a bullet checklist or a table with columns like:
- Step
- Status
- Notes

Example structure:
- 收到新人到職通知 — completed / pending — notes
- 建立公司信箱 — completed / pending — notes
- 加入 Slack 頻道 — completed / pending — notes
- 準備筆電與門禁卡 — completed / pending — notes
- 安排第一週訓練 — completed / pending — notes
- 指派導師 — completed / pending — notes
- 第 30 天面談 — completed / pending — notes
- 記錄到人資系統 — completed / pending — notes

## Quality rules
- Preserve the exact step order.
- Keep terminology aligned with the confirmed diagram.
- Never add steps that are not in the confirmed process.
- Never convert the confirmed sequence into a branching workflow.
- When the input is incomplete, explicitly report the gap and continue listing the confirmed steps when possible.