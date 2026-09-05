---
name: new-hire-onboarding-flow
description: Use this skill when you need to turn a confirmed new-hire onboarding flow into a portable, step-by-step execution guide without inventing extra branches or steps.
---

# New Hire Onboarding Flow

## Purpose
Use this skill to convert a confirmed onboarding flow into a clear execution guide for agents. Apply it when the input is a verified process diagram or a previously confirmed sequence of onboarding steps.

## Input assumptions
- The flow has already been confirmed.
- The sequence contains these exact nodes:
  1. 收到新人到職通知
  2. 建立公司信箱
  3. 加入 Slack 頻道
  4. 準備筆電與門禁卡
  5. 安排第一週訓練
  6. 指派導師
  7. 第 30 天面談
  8. 記錄到人資系統
- No additional branches, conditions, or hidden steps are to be inferred.

## Instructions
1. Start from the first confirmed node and present each node in order.
2. For each node, describe the action plainly and keep the wording aligned with the confirmed node name.
3. Preserve the original order of the flow.
4. Do not add decision points, exceptions, owners, deadlines, or substeps unless they are explicitly present in the confirmed input.
5. If the input is missing or changes, stop and ask for confirmation before revising the skill.

## Output format
- Produce a compact onboarding guide with one section per confirmed node.
- Use the confirmed node names as the section headings or bullet labels.
- Keep the output portable and free of environment-specific assumptions.

## Quality checks
- All eight confirmed nodes are present.
- No extra nodes appear.
- No branches or conditions are introduced.
- The final text is suitable for reuse as a portable agent skill.