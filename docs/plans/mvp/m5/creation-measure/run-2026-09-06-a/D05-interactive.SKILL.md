---
name: new-hire-onboarding-flow-skill
description: 將已確認的新人體驗到職流程圖整理成可重用的 Agent Skill，適合在需要把流程節點轉成正式技能規格與操作說明時使用。
---

# New Hire Onboarding Flow Skill

## Purpose
This skill turns a confirmed onboarding flow into a portable skill specification and operating guide. Use it when you need to document or reuse a newcomer onboarding process without inventing missing steps, conditions, or responsibilities.

## Inputs
- A confirmed understanding of the onboarding flow nodes.
- Any explicitly confirmed conditions, branches, roles, inputs, or completion criteria.
- A list of known uncertainties that must remain labeled as unknown.

## Output
Produce a complete skill package with:
- `name`
- `description`
- `compatibility`
- `allowed_tools`
- `body`
- optional files only if a script is genuinely better than instructions

## Operating Rules
1. Preserve the confirmed sequence and relationships between nodes.
2. Do not add branches, decision points, roles, or completion rules that are not explicitly confirmed.
3. Keep uncertainties visible as uncertainties; never rewrite them as facts.
4. If no tools are required, leave `allowed_tools` empty.
5. Do not reference external sources unless they are explicitly provided and confirmed.

## Confirmed Nodes
- Receive onboarding notification
- Create company email account
- Add the new hire to Slack channels
- Prepare laptop and access card
- Arrange first-week training
- Assign a mentor
- Hold the day-30 check-in
- Record completion in the HR system

## Known Uncertainties
- No branches or decision conditions are shown in the diagram.
- Roles, inputs, and completion criteria are not labeled.
- The target department or employee type is not specified.

## How to Use This Skill
1. Start from the confirmed nodes only.
2. Verify whether any new information is confirmed before adding it.
3. Keep the output concise and structured so it can be reused as a portable skill.
4. If the source flow changes, update the confirmed understanding before revising the skill.

## Validation Checklist
- The skill names the onboarding flow specifically.
- The documented nodes match the confirmed understanding.
- No extra branches or conditions appear.
- Uncertainties remain labeled as unknown.
- Tool requirements stay empty unless explicitly required.