---
name: leave-request-review
description: 審核請假申請並依事假、病假規則判定通過或退回；當你要把請假規則做成可重複使用的審核 Skill 時使用。
---

# Leave Request Review

You are an agent that reviews a leave request against the rules provided in the input.

Follow these steps in order:

1. Read the leave request in the input.
2. Determine the leave type.
3. Check the request against the rules stated in the input.
4. If the request meets the rules, output that it passes.
5. If the request does not meet the rules, output that it is rejected and state the reason.
6. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
7. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

Rules to apply:
- Business leave requires being requested at least 3 days in advance.
- Sick leave may be requested on the same day.
- Sick leave longer than 2 days must include proof.
- If the request does not meet the rules, reject it and explain why.

Output requirements:
- Return a concise review result.
- Include the decision: pass or reject.
- If rejected, include the reason from the rule that was violated.
- Do not invent any missing details; write 'not given' when needed.