---
name: slack-weekly-unanswered-question-list
description: Extract unanswered question messages from three specified Slack channels and produce a weekly list with links. Use when you receive a Slack export or weekly message digest and need a concise triage summary.
---

# Slack Weekly Unanswered Question List

## Purpose
Turn a weekly Slack digest or export into a clean list of unanswered question messages from exactly three specified channels, with a link for each item.

## Input
The input should contain:
- the names of exactly three Slack channels to inspect
- the week's messages for those channels
- for each message, enough information to judge whether it is a question, whether it has replies, and its message link

If the input already includes a clear weekly window, use that window. If it does not, treat the provided batch as the weekly set.

## What to extract
Include a message only when all of the following are true:
1. it comes from one of the three specified channels
2. it is phrased as a question or request for help
3. it has no replies
4. it has a message link

Exclude:
- messages from any other channel
- answered or replied-to messages
- announcements, confirmations, or status updates that are not questions
- items without a usable link

## How to work
1. Read the three channel names from the input and treat them as the only allowed sources.
2. Scan each message in those channels.
3. Decide whether the message is a question or help request.
4. Check whether it has replies. Any reply count above zero means it is not included.
5. Collect the message text, channel, author if present, timestamp if present, and link.
6. Sort the final list in the same order the messages appear in the input, unless the input explicitly asks for a different order.
7. Produce a weekly summary containing only the matching items.

## Output format
Return a Markdown list with one item per message.

Use this structure:
- **Channel** — **Author** — message text
  - Link: URL
  - Time: timestamp, if provided

If there are no matches, return:

```text
本週沒有未回覆的問題訊息。
```

## Style rules
- Be concise.
- Do not add commentary about how the result was computed.
- Do not mention excluded messages.
- Do not invent links or reply counts.
- Keep the output focused on the weekly unanswered-question list.

## Reliability rules
- When the input is ambiguous about whether a message is a question, prefer inclusion only if the message clearly seeks help or information.
- When the input is ambiguous about reply count, treat an explicit reply indicator as authoritative.
- When the input is ambiguous about channel membership, include only messages explicitly under one of the three specified channels.