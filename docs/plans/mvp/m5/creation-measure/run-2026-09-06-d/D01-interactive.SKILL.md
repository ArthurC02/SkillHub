---
name: workflow-to-skill-drafter
description: 把已確認的中文流程圖理解轉成結構化的 Agent Skill 草稿；當你需要將流程圖整理成可移植的技能規格、並保留不確定處時使用。
---

# Workflow to Skill Drafter

## Purpose
This skill helps turn an already-confirmed Chinese flowchart understanding into a portable Agent Skill draft. Use it when you need to convert a workflow into a structured skill specification while preserving confirmed assumptions and explicitly marking uncertainties.

## What to produce
When given the confirmed brief and diagram understanding, produce a Skill package with:
- a clear task description
- the expected inputs
- the expected outputs
- tool requirements, if any
- limitations and uncertainty handling
- a complete `SKILL.md` body written in concrete steps

## Operating rules
1. Start from the confirmed brief and confirmed diagram understanding. Do not rewrite their core meaning.
2. Keep the scope narrow: draft the skill that helps organize and write the final skill from the confirmed workflow understanding.
3. Do not invent tools, trial results, or hidden workflow steps.
4. If a workflow branch is unclear, preserve it as an uncertainty instead of resolving it silently.
5. If essential information is still missing, ask a short, answerable clarification question rather than guessing.

## Drafting procedure
1. Restate the task in one sentence using the confirmed brief.
2. Summarize the available inputs:
   - the confirmed brief
   - the confirmed diagram understanding
   - any user-provided constraints
3. Describe the output format expected from the drafting process.
4. List tool requirements only if a specific tool is truly needed.
5. Include limitations:
   - no unconfirmed tools
   - no fabricated trial success
   - no silent resolution of diagram ambiguities
6. Write steps that an agent can follow to create or revise the draft.

## Diagram handling guidance
When the diagram contains branches:
- name each node explicitly
- state the condition attached to each branch
- map the observable branch outcomes
- separate confirmed branches from uncertainties
- if a branch re-joins later steps, mention the rejoin only if it is explicitly supported by the confirmed understanding

## Quality checklist
Before finalizing a draft, verify that:
- the task matches the confirmed brief
- the inputs are named clearly
- the outputs are described in an observable way
- tool requirements are minimal and accurate
- limitations mention uncertainty handling
- no unconfirmed detail is presented as fact

## Revision behavior
If validation reveals a mismatch, revise the draft by:
- tightening the description of the task
- removing unsupported tool claims
- restoring any omitted uncertainty
- keeping the confirmed brief and diagram understanding intact

## Output expectation
The final Skill should read as a practical, portable instruction set for an agent that must draft a Skill from a confirmed workflow understanding without over-assuming context.