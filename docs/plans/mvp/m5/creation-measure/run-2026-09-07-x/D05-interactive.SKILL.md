---
name: new-hire-onboarding-skill
description: 將新人到職流程圖轉成可直接實作的 Agent Skill；當你要把已確認的到職節點整理成可移植、不可自行加枝節的技能規格時使用。
---

# New Hire Onboarding Skill

Use this skill when you need to turn a confirmed new-hire onboarding flow into a portable Agent Skill specification.

## Instructions

1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Work through the confirmed nodes in order, and do not add any step, condition, role, or tool that is not present in the confirmed flow.
4. If the input is missing required details for the artifact, state 'not given' for those details instead of inventing them.
5. Keep the output self-contained and directly usable as the finished skill specification.

## Confirmed flow nodes

1. 收到新人到職通知
2. 建立公司信箱
3. 加入 Slack 頻道
4. 準備筆電與門禁卡
5. 安排第一週訓練
6. 指派導師
7. 第 30 天面談
8. 記錄到人資系統

## What to produce

Produce a portable Skill draft that includes:
- the skill name
- the skill description
- compatibility
- allowed tools
- the full instruction body
- files only if a script genuinely does the work better than instructions

If a field is not given in the input, write 'not given'.

## Workflow

Follow the confirmed nodes exactly in order:

1. 收到新人到職通知
2. 建立公司信箱
3. 加入 Slack 頻道
4. 準備筆電與門禁卡
5. 安排第一週訓練
6. 指派導師
7. 第 30 天面談
8. 記錄到人資系統

Do not introduce any branching logic, exception path, or extra step. Where the input is silent, say 'not given'.