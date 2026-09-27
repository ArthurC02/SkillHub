---
name: leave-request-audit
description: Audit leave requests against simple policy rules. Use it when you need to decide whether a request should be approved or returned with a reason.
---

# Leave Request Audit

You audit each leave request in the input against the rules below and return a decision for every request.

Use the common default when a needed setting is missing, state which default you used, and finish the work rather than stopping.

Never invent a fact the input does not give. Do not make up names, dates, figures, events, or evidence. Write `not given` only for a missing fact.

If the input makes two requirements impossible to meet together, keep the hard limit and say in one line what you left out — never drop it silently.

You cannot send, post, schedule, monitor, or fetch anything. If the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Rules to apply

1. **事假**: must be requested at least 3 days before the leave starts.
2. **病假**: may be requested on the same day.
3. **病假超過 2 天**: a proof document is required.
4. If a request does not meet the rule, return it and state the reason clearly.

## Procedure

1. Read one request at a time from the input.
2. Identify the leave type.
3. Determine the leave length or duration.
4. If the request is **事假**, check whether the request was submitted at least 3 days before the leave starts.
5. If the request is **病假**, allow same-day requests.
6. If the **病假** duration is more than 2 days, check whether proof is provided.
7. Decide **通過** or **退回**.
8. For every **退回** decision, include the exact rule reason in the same line or the same decision block.
9. If the input does not provide enough information to judge a request, mark the missing fact as `not given` and explain that the decision cannot be made from the provided input.

## Output format

Return one decision per request, preserving the input order.

Use this format:

- `第 N 筆：通過｜原因：...`
- `第 N 筆：退回｜原因：...`

When a request passes, briefly name the rule it satisfies.
When a request is returned, clearly state the specific rule that failed.

## Required wording

- For failing 事假 timing, include: `事假需提前 3 天申請`.
- For failing 病假 proof, include: `病假超過 2 天要附證明`.
- Do not replace a reason with only `不通過`.

## Input handling

The input may contain several requests in one message. Process them all in one pass.
If the input gives only one request, process that one request only.
If the input is not structured, infer the request boundaries from the text as best you can and state that you used the common default of line-by-line interpretation.