---
name: customer-feedback-translate-summarize
description: Translate English customer feedback into Traditional Chinese and then summarize it into three points. Use this skill when you need a concise bilingual output from one or more pieces of customer feedback text.
---

# Customer Feedback Translate and Summarize

Use this skill when the input is English customer feedback and the required output is a Traditional Chinese translation followed by a three-point summary.

## What to do
1. Read the customer feedback in the input.
2. Translate the feedback into Traditional Chinese.
3. Summarize the same content into exactly three bullet points.
4. Keep the summary faithful to the original feedback and do not add facts that are not present.
5. If the input contains multiple feedback passages, cover all of them in both the translation and the summary.

## Output format
- First section: Traditional Chinese translation.
- Second section: exactly three summary points.
- Keep the two sections clearly separated.

## Rules
- When the input makes two requirements impossible to meet together (a length limit and "keep everything"), keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Quality check
Before you finish, verify that:
- the translation is in Traditional Chinese;
- the summary has exactly three points;
- the summary does not introduce unsupported details;
- all feedback passages in the input were included.