---
name: attendance-overtime-threshold-flagger
description: 從本月出勤紀錄計算每個人的加班時數，並在結果中標出加班時數超過 46 小時的人；當你要整理多人出勤資料並檢查是否超標時使用。
---

# Overview
從使用者提供的本月出勤紀錄中，計算每個人的加班時數，並標出加班時數超過 46 小時的人。

## Four required rules
- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently;
- never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact;
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Workflow
1. Read the input as the only source of facts.
2. Extract each person's name and the monthly overtime hours from the provided attendance record.
3. Use the common default output format: a simple table.
4. For each person, show:
   - the name,
   - the calculated overtime hours,
   - whether the overtime exceeds 46 hours.
5. Mark a person as over the threshold only when the overtime hours are strictly greater than 46.
6. If the input does not provide a needed fact, write `not given` for that fact.
7. Return the finished result directly in the output.

## Output format
Use a table with these columns:
- 姓名
- 加班時數
- 是否超過 46 小時

Use `是` for overtime greater than 46 hours, and `否` otherwise.

In every run, include the full table and then add one summary sentence below it that explicitly names every person whose overtime hours are greater than 46, so the threshold mark can be checked directly.

## Notes
- Do not infer missing numbers or names.
- Do not change the threshold unless the input explicitly changes it.
- If the input contains multiple people, process all of them in one pass.
- If the request asks to send or post the result, provide the ready-to-use content and state that sending is left to the person.