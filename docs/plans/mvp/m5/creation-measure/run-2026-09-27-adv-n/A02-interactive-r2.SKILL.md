---
name: leave-request-review
description: 審核請假申請，當輸入事假或病假申請時判定是否通過、需補件或退回並說明原因；適合用在需要依提前申請天數與證明條件快速產出一致審核結果的情境。
---

# Leave Request Review

## Purpose
Review leave requests and return a clear decision for each request: approve, return for revision, or reject with reasons.

Use this skill when the input is one or more leave-request texts and you need a consistent decision based on the request type, request timing, leave duration, and whether supporting proof is attached.

## Required inputs
For each request, identify as much of the following as the input provides:
- leave type
- request date
- leave start date
- leave end date or number of leave days
- proof attachment status, when relevant

If the input is missing needed details, use the common default of a conservative review outcome: return the request and say which fields are missing. State that this default was used.

## Decision rules
1. **Personal leave** must be requested at least 3 days in advance.
   - If the request is made fewer than 3 days before the leave starts, return the request with the reason that personal leave must be requested 3 days in advance.
2. **Sick leave** may be requested on the same day.
   - If sick leave is longer than 2 days, proof is required.
   - If proof is missing for sick leave longer than 2 days, return the request with the reason that proof is required.
3. If the input does not provide enough information to apply the rules, return the request and name the missing information.
4. If multiple requests are included, evaluate each request separately and give each one its own decision and reason.

## Operating instructions
- Process every request in the input in one pass.
- Do not ask follow-up questions unless the input is unusable and no conservative default can be applied; in that case, return a template of the missing fields and note what to fill in.
- Never invent facts that the input does not provide. Use **not given** only for missing facts.
- When a setting needed to do the work is missing, use the common default, say which one you used, and finish the work rather than stopping.
- If the input makes two requirements impossible to satisfy together, keep the hard limit and say in one line what you left out — never drop it silently.
- You cannot send, post, schedule, monitor or fetch anything, so if the request asks for that, prepare the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output format
Return a concise, structured result for each request.
Suggested format:
- Request 1: Approved / Returned / Rejected
  - Reason: ...
- Request 2: Approved / Returned / Rejected
  - Reason: ...

Keep the wording clear and specific. If a request is rejected or returned, the reason must explain the exact rule that was not met.

## Default interpretation guidance
If the input states dates but does not explicitly say the number of days, compute the duration from the leave start and end dates.
If the input gives a date range and a request date, use the gap between the request date and the leave start date for the advance-notice check.
If proof is mentioned as yes/no, treat yes as attached and no as not attached.