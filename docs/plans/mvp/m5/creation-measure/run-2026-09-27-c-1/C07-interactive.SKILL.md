---
name: travel-expense-reimbursement-checker
description: 核對員工出差報帳明細，逐日加總花費並比對員工填寫總計是否一致；當你需要檢查多天報帳、找出加總差異或產出可直接回覆的核對結果時使用。
---

# Purpose
Check business-trip reimbursement details by day, sum the listed expenses for each day, and compare each daily total against the employee-provided total.

Use this skill when you receive a reimbursement draft, expense log, or trip report that includes dated daily items and a claimed total amount.

# Inputs
Expect the user input to contain, for each day:
- a date or day label
- one or more expense lines with amounts
- a claimed total for that same day

If the input does not explicitly provide a date, treat each clearly separated block as one day and label it as “未提供日期” in the output.
If the input does not explicitly provide a claimed total, report that the employee total is not given and continue with the daily sum.

# Procedure
1. Split the input into daily blocks.
2. For each day, extract every expense amount.
3. Sum the amounts for that day.
4. Read the employee-provided total for that day.
5. Compare the computed sum with the provided total.
6. State whether the day is一致 or 不一致.
7. If there are multiple days, also provide an overall summary of how many days matched and how many did not.

# Output format
Return a clear result with these parts:
- 一覽: list each day, its computed sum, and its claimed total
- 比對結果: say whether each day matches or mismatches
- 整體結論: report the total number of days checked, the number一致, and the number不一致

When amounts that belong together are listed, include their total.
For example, if three expense lines are 120, 350, and 180, show the total as 650.

# Rules
- Never invent a date, amount, or expense item that the input does not give.
- If a required fact is missing, mark it as not given in the same language as the output.
- Use the common default of treating the provided numbers as the complete set for that day unless the input says otherwise.
- If one day’s sum does not match the claimed total, report the difference explicitly.
- If the input contains more than one day, check every day in the input.
- Do not decide whether an expense is reimbursable; only sum and compare.

# Response behavior
Produce the finished reimbursement check in the answer itself.
Do not ask follow-up questions unless the input is too incomplete to identify any day or any amount.
When the output is constrained by a hard limit and “keep everything” at the same time, keep the hard limit and say in one line what was left out.
You cannot send, post, schedule, monitor, or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
