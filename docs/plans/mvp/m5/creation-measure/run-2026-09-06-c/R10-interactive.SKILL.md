---
name: slack-unanswered-questions-weekly
description: Scan three specified Slack channels each week to find unanswered questions and produce a linked list. Use when you need a recurring review of open questions in a small set of Slack channels.
---

# Slack unanswered questions weekly

## What this skill does
Each week, review exactly three specified Slack channels, identify messages that are questions and have not been answered, and output a list with links to the original messages.

## When to use it
Use this skill when a recurring weekly check is needed for a small set of Slack channels and the result should be a concise review list for follow-up.

## Inputs required
- The three Slack channel names or channel IDs.
- The rule for deciding whether a question is “unanswered”.
- The preferred output format, if any.

If any of these are missing, ask for them before proceeding.

## Procedure
1. Access the three specified Slack channels.
2. Review messages from the relevant weekly period.
3. Identify messages that are questions or request help.
4. Determine whether each one is unanswered using the confirmed rule.
5. For each included item, capture:
   - channel
   - question text
   - author
   - timestamp
   - message link
   - reason it is considered unanswered
6. If a message cannot be confidently classified, mark it as “待確認” and do not treat it as unanswered.
7. Produce the weekly list in a consistent format that can be copied into a report or notification.

## Output requirements
- Include only messages from the three specified channels.
- Include only items judged unanswered, plus any ambiguous items clearly marked as “待確認” when the rule cannot be applied confidently.
- Every item must include a link to the original Slack message.
- Keep the format stable across weekly runs.

## Classification guidance
- Use the confirmed unanswered rule exactly as provided.
- Do not infer a reply when there is insufficient evidence.
- Prefer explicit evidence from the Slack thread or conversation context.
- If the reply status is unclear, mark the item as needing human review.

## Recommended output shape
A simple list or table is acceptable, as long as it is consistent. Each row or bullet should contain:
- channel
- question
- author
- time
- link
- unanswered reason or “待確認”

## Validation check
Before finalizing, verify that:
- only three channels were used;
- the output contains links for every item;
- unanswered items follow the confirmed rule;
- ambiguous cases are marked “待確認”.
