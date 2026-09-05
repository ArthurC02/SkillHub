---
name: reimbursement-flowchart-parser
description: Parse an employee reimbursement flowchart into a structured summary with steps, conditions, branches, and uncertainties. Use this skill when given a reimbursement process diagram or text transcription and you need a faithful, step-by-step interpretation without inventing missing branches.
---

# Reimbursement Flowchart Parser

## Purpose
This skill turns an employee reimbursement flowchart into a structured process summary. Use it when you are given a flowchart, diagram transcription, or step list for reimbursement and need a faithful breakdown of nodes, decision points, branches, and ambiguities.

## What to extract
Read the input and identify:
- **Nodes**: the process steps or actions.
- **Conditions**: every decision question or test.
- **Branches**: the outcomes for each condition, including the path that follows.
- **Uncertainties**: anything the diagram does not make explicit.

## How to work
1. Read the diagram or transcription as given.
2. List each step in process order.
3. Separate decision points from action nodes.
4. For each decision, capture each branch exactly as shown.
5. Do not invent missing paths, exceptions, approvals, retries, or end states.
6. If a step is implied but not explicit, mark it as an uncertainty instead of filling it in.
7. If the transcription is incomplete or ambiguous, say so clearly.

## Output format
Return a structured summary with these sections:
- **Nodes**
- **Conditions**
- **Branches**
- **Uncertainties**

Use concise bullets. Keep wording close to the source text. Preserve names such as:
- `發票是否齊全`
- `金額是否超過5000`
- `退回並要求3天內補件`
- `送經理簽核`
- `財務直接入帳`
- `寄出付款通知信`

## Fidelity rules
- Do not add new steps unless they are explicitly present in the source.
- Do not assume what happens after a rejection, correction, or handoff unless the source says so.
- Do not infer hidden approval rules, SLA timing, or exception handling.
- If the input mentions a branch but not its destination, report that as an uncertainty.
- If two labels appear similar, keep the exact wording from the source and avoid normalization that changes meaning.

## Handling the sample reimbursement flow
For a flow like:
- Start -> `收到員工報帳申請` -> `判斷發票是否齊全`
- No: `退回並要求3天內補件`
- Yes: `判斷金額是否超過5000`
  - Yes: `送經理簽核` -> `財務直接入帳` -> `寄出付款通知信`
  - No: `財務直接入帳` -> `寄出付款通知信`

A correct summary must preserve both conditions and their branches, and must not invent any extra path for the补件 step, manager rejection, or post-payment exceptions.

## Quality checklist
Before finishing, verify that:
- every explicit condition is listed,
- every explicit node is represented,
- branch directions match the source,
- uncertainties are called out instead of guessed.