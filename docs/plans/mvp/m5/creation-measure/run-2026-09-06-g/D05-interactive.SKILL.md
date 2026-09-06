---
name: onboarding-flow-skill
description: Use this skill when you need a clean, portable Agent Skill that turns a confirmed employee onboarding flowchart into a single-path operational workflow. It is suitable when the source diagram has already been confirmed and you want the process captured without adding extra branches, conditions, owners, or tools.
---

# Onboarding Flow Skill

## Purpose
Turn the confirmed onboarding flow into a portable Agent Skill that can be reused as-is.

## What this skill does
- Reads a confirmed onboarding process as a single path.
- Preserves only the steps that are explicitly present in the source flow.
- Writes the process in a clear operational order from start to finish.
- Avoids inventing branches, conditions, owners, outputs, or tool steps that are not shown.

## When to use this skill
Use this skill when the input is a confirmed onboarding flowchart or an equivalent step list and you need a reusable Skill package that captures the flow faithfully.

## Instructions
1. Identify the confirmed onboarding steps in order.
2. Rewrite the flow as a single linear procedure.
3. Keep the wording close to the confirmed step names, while making the sequence readable as instructions.
4. Do not add decision points, exceptions, fallback paths, or conditional logic unless they are explicitly present in the confirmed source.
5. Do not assign responsibility, systems, or outputs unless they are explicitly shown in the confirmed source.
6. Present the result as a Skill package with a clear title, a short description, and a concise body that an agent can follow in one pass.

## Confirmed flow to preserve
1. 收到新人到職通知
2. 建立公司信箱
3. 加入 Slack 頻道
4. 準備筆電與門禁卡
5. 安排第一週訓練
6. 指派導師
7. 第 30 天面談
8. 記錄到人資系統

## Output behavior
When given this confirmed flow, produce the onboarding process as a straightforward linear sequence in the same order, ending with the HR system record step.

## Constraints
- Do not add branches or conditions.
- Do not add steps that are not in the confirmed flow.
- Do not assume owners, tools, or deliverables unless the source explicitly includes them.
- Keep the result portable and self-contained.