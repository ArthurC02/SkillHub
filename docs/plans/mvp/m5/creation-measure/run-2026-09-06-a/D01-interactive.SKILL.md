---
name: employee-expense-flow-skill
description: 將員工報帳申請流程圖整理為結構化流程說明，適合在已確認圖意、需要產出可攜式 Agent Skill 草稿時使用。
---

# employee-expense-flow-skill

## Purpose
Convert a confirmed employee expense reimbursement flowchart into a clear, structured Skill draft. Use this skill when the input is a verbal or textual description of the flowchart, and the goal is to preserve the confirmed nodes, conditions, branches, and uncertainties without inventing missing logic.

## What to produce
Produce a Skill draft that:
- lists the confirmed process steps in order;
- states decision conditions explicitly;
- shows each branch and its result;
- preserves any uncertainties as uncertainties, not as facts;
- avoids adding extra nodes or rules that were not confirmed.

## Workflow
1. Read the confirmed flow understanding.
2. Extract the nodes in the order implied by the confirmed process.
3. List each condition as a decision point.
4. Map each branch to the next node or outcome.
5. If a step is uncertain, label it as uncertain and explain the ambiguity briefly.
6. Draft the Skill using only confirmed information.

## Drafting rules
- Do not infer hidden steps, exceptions, or alternate routes unless they are explicitly confirmed.
- If a transition is ambiguous, state the ambiguity instead of resolving it.
- Keep the final Skill portable and self-contained.
- Use concise, user-facing language that can be reused as a Skill package.

## Recommended output structure
- Title
- Trigger / when to use
- Input expectations
- Process summary
- Conditions and branches
- Uncertainties
- Validation notes

## Validation checklist
Before finalizing, verify that:
- all confirmed nodes appear in the draft;
- every confirmed condition is represented;
- every confirmed branch is included;
- no unconfirmed nodes or rules were added;
- uncertainty language is preserved exactly where needed.
