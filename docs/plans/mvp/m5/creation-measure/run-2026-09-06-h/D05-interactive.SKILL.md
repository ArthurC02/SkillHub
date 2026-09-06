---
name: new-hire-onboarding-flow
description: Use this skill when you need to run or describe a standard new-hire onboarding workflow from a single confirmed sequence. It turns the provided onboarding notes into a portable skill that follows the exact ordered steps with no added branches.
---

# New Hire Onboarding Flow

Use this skill when you are given a confirmed, single-path onboarding sequence and need to apply it without adding extra steps, branches, or assumptions.

## Goal

Execute the onboarding workflow exactly as provided in the confirmed sequence.

## Instructions

1. Start with **收到新入職通知**.
   - Treat the provided input as the onboarding trigger.
   - Do not infer any additional approval, routing, or intake steps.

2. Continue to **建立公司信箱**.
   - Create or describe the company email setup from the input you are given.
   - If the input does not contain enough detail to do this concretely, state that the detail is missing rather than inventing it.

3. Continue to **加入 Slack 頻道**.
   - Add the new hire to the relevant Slack channel(s) if those channel names are present in the input.
   - If channel names are not provided, report that the specific channel information is missing.

4. Continue to **準備筆電與門禁卡**.
   - Prepare the laptop and access card as requested by the input.
   - Do not add procurement, budgeting, or approval subflows unless they are explicitly included in the input.

5. Continue to **安排第一週訓練**.
   - Schedule the first-week training based on the provided material.
   - If dates, owners, or topics are missing from the input, state that those details are missing.

6. Continue to **指派導師**.
   - Assign a mentor using the information in the input.
   - Do not invent a mentor, selection rule, or backup path if the input does not specify one.

7. Continue to **第 30 天面談**.
   - Prepare or describe the 30-day review using the provided input.
   - If the input does not specify participants, agenda, or timing details, say so directly.

8. Finish with **記錄到人資系統**.
   - Record the onboarding outcome in the HR system using only the information present in the input.
   - Do not fabricate fields, system names, or statuses that are not supplied.

## Output rules

- Follow the eight steps in the exact order listed above.
- Treat this as a single-path workflow; do not create conditions, branches, or alternate routes.
- Use only facts present in the input.
- When required details are missing from the input, explicitly state that they are missing instead of guessing.
- Do not add any onboarding steps beyond the confirmed sequence.

## When to use

Use this skill whenever a user provides a confirmed onboarding sequence and wants the workflow applied, summarized, or packaged into a reusable skill.