---
name: hr-onboarding-flow-skill
description: 將已確認的入職流程節點整理成可重複使用的結構化技能文件；當你需要把 onboarding 流程圖轉成可執行的 Agent Skill，且不補寫未出現的分支時使用。
---

# HR Onboarding Flow Skill

## Purpose
Use this skill when you are handed a confirmed onboarding flow node list and need to turn it into a reusable, structured Agent Skill document.

This skill is limited to the confirmed flow below and must not add branches, conditions, exceptions, or extra roles.

## Confirmed workflow nodes
1. 收到新入到職通知
2. 建立公司信箱
3. 加入 Slack 頻道
4. 準備筆電與門禁卡
5. 安排第一週訓練
6. 指派導師
7. 第 30 天面談
8. 記錄到人資系統

## What to produce
Create a complete skill package that explains:
- what the skill is for
- when to use it
- the exact onboarding nodes in order
- how to format the input
- how to format the output
- the limits of the skill

## Required behavior
When drafting the skill:
1. Preserve the confirmed node order exactly as given.
2. Describe each node plainly and only as part of the confirmed workflow.
3. Keep the skill reusable by stating that it operates on a confirmed onboarding node list.
4. State that no extra branches or conditions are introduced.
5. State that the skill does not invent missing responsibilities, exceptions, or checks.

## Input
Accept a plain-text list of onboarding steps that matches the confirmed node sequence.

## Output
Return a structured skill document in Markdown with:
- a clear title
- a short description that includes the trigger and use case
- workflow steps
- input/output expectations
- limitations

## Constraints
- Do not add branching logic.
- Do not infer missing conditions.
- Do not add exception handling.
- Do not add role assignments not present in the confirmed nodes.
- Do not ask the user to fill in missing process details.
- If the provided input does not match the confirmed node set, report that the input is outside the confirmed scope and stop.

## Suggested structure for the generated skill
- Title
- Description
- When to use
- Confirmed workflow nodes
- Input specification
- Output specification
- Limitations

## Quality check
Before finishing, verify that:
- all 8 nodes are present
- the order is unchanged
- no branch or condition was added
- the skill text clearly states the confirmed scope only
