---
name: new-hire-onboarding-flow-skill
description: 將新人到職流程圖整理成可重複使用的 Agent Skill；當你需要把確認過的 onboarding 節點轉成一份 portable Skill 時使用。
---

# Purpose
This Skill turns a confirmed onboarding flow diagram into a portable Agent Skill. Use it when you need to convert a verified new-hire process into a reusable skill description without adding unsupported steps or branches.

# Instructions
1. Read the confirmed flow nodes exactly as provided.
2. Preserve the node order exactly as confirmed.
3. Write the skill as a portable, reusable instruction set.
4. Do not add steps, decisions, branches, roles, or tools that are not explicitly confirmed.
5. State explicitly that no additional tools are required.
6. If the confirmed input is incomplete, use only the confirmed content and stop there.

# Confirmed flow
1. 收到新人到職通知
2. 建立公司信箱
3. 加入 Slack 頻道
4. 準備筆電與門禁卡
5. 安排第一週訓練
6. 指派導師
7. 第 30 天面談
8. 記錄到人資系統

# Output requirements
- Output a Skill that reflects the confirmed flow in the same order.
- Keep the wording faithful to the confirmed nodes.
- Include no extra workflow branches, fallback paths, or unconfirmed implementation details.
- Explicitly note that no extra tools are required.

# Completion check
Before finalizing, verify that the resulting Skill:
- uses only the confirmed nodes,
- keeps their order,
- avoids adding branches or conditions,
- states that no additional tools are needed,
- and remains usable as a portable Agent Skill.