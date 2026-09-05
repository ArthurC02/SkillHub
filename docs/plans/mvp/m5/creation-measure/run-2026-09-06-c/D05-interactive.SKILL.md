---
name: new-hire-onboarding-flow
description: Use this skill when you need to turn an approved HR onboarding flow into a clear, step-by-step agent procedure or checkable output. It helps agents handle new-hire setup work without inventing missing steps or branches.
---

# New Hire Onboarding Flow

## Purpose
Use this skill when you need to turn an approved HR onboarding flow into a clear, step-by-step agent procedure or checkable output.

## When to use
- A new hire has been announced and the onboarding sequence must be followed.
- You need a portable instruction set that mirrors the approved flow.
- You want the agent to avoid inventing extra steps, conditions, or branches.

## Inputs
- The confirmed onboarding flow nodes.
- Any additional onboarding details only if they are required to complete the task.

## Core rules
1. Follow only the confirmed flow nodes.
2. Do not add branches, conditions, or hidden steps that are not confirmed.
3. If a needed detail is missing, ask for it instead of guessing.
4. Keep outputs concrete, ordered, and checkable.

## Procedure
1. Receive the new-hire onboarding notification.
2. Create the company email account.
3. Add the new hire to the Slack channel.
4. Prepare the laptop and access badge.
5. Arrange first-week training.
6. Assign a mentor.
7. Hold the day-30 review meeting.
8. Record completion in the HR system.

## Output format
When acting on this skill, return a concise onboarding checklist or status summary that:
- lists the confirmed steps in order,
- marks completed and pending items clearly,
- notes any missing information as a question,
- avoids any unconfirmed assumptions.

## Validation checklist
Before finalizing, verify that:
- every step maps to a confirmed node,
- no extra steps were introduced,
- any missing input was requested,
- the output is easy to reuse in future onboarding cases.