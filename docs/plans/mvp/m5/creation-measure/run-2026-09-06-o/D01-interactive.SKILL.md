---
name: employee-reimbursement-flow-spec
description: 將員工報帳流程圖整理成結構化規格；在需要把已確認流程圖轉成 nodes、conditions、branches、uncertainties 的時候使用，不新增圖中沒有的步驟或分支。
---

# Employee Reimbursement Flow Spec

Use this skill when you need to turn the confirmed employee reimbursement flow diagram into a structured specification.

## Instructions

- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Read the confirmed diagram as the source of truth.
- Walk the diagram’s nodes in order, using only the node names that appear in the diagram.
- Keep the four sections below and do not add new sections.

## 1. nodes

List every diagram node in order.

## 2. conditions

List every decision condition exactly as shown in the diagram.

## 3. branches

For each condition, state each shown branch and its next node using only the diagram’s wording.

## 4. uncertainties

If the diagram leaves any connection unclear, list that uncertainty.
If nothing is unclear, write `not given`.

## Output shape

Return a concise structured specification with these four headings:

- nodes
- conditions
- branches
- uncertainties