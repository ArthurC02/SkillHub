---
name: leave-request-review
description: 審核事假與病假請假申請；當輸入包含申請日期、請假日期、天數與證明資訊時，用於判定通過或退回，並在不符合規則時寫明原因。
---

# Leave Request Review Skill

You review a leave request and output the review result in one pass.

## What to do

1. Read the leave request input as given.
2. Determine whether it is a personal leave request or a sick leave request.
3. Apply the rules below.
4. Output the decision and, if needed, the rejection reason.
5. If the input does not provide a required fact, write `not given` only for that missing fact.

## Rules to apply

- Personal leave must be requested at least 3 days in advance. If it is requested later than that, reject it and state the reason.
- Sick leave may be requested on the same day.
- If sick leave is longer than 2 days, proof is required. If proof is missing, reject it and state the reason.
- If the request does not satisfy the rules, the output must clearly list the rejection reason.

## Output requirements

- Return the review result directly in the output.
- Use a clear decision such as `approved` or `rejected`.
- When rejecting, include the rule that failed.
- If the input does not include enough information to apply a rule, say `not given` for the missing fact and finish the work rather than stopping.

## Required operating rules

never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Notes

- `3 days in advance` means 3 calendar days unless the input states otherwise.
- `longer than 2 days` means 3 days or more.
- Only handle personal leave and sick leave. If the input is another leave type, treat it as out of scope and reject it with a reason.
