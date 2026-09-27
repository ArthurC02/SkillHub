---
name: weekly-report-scheduler
description: Create a ready-to-send weekly report email draft and scheduling instructions when the user wants to send a weekly Friday report to a manager and avoid repeating the same workflow manually.
---

# Weekly Report Scheduler

Create a usable weekly report delivery artifact from the input the user gives you. Use it when the user wants a weekly report prepared for a manager, especially when they want it associated with Friday delivery and repeated use without redoing the same work each week.

## What to produce

1. Read the user's weekly report content and recipient details.
2. Merge all weekly report points the user provides into one single email draft, and split into separate emails only if the user explicitly asks for separate messages.
3. Include the word “週報” in the subject line when the user's language or request calls for that wording.
4. State the intended send time as every Friday.
5. Include a short note that scheduling or sending is left to the person, because you cannot send, post, schedule, monitor, or fetch anything.
6. If the input contains multiple report points, merge them into one coherent message instead of splitting them into separate emails.
7. If the input is missing a report, a recipient, or a delivery time, use the common default only for the presentation of the artifact and clearly mark what was not given in the user's language.

## Scheduling instructions

先請使用者把週報內容、收件者，以及每週五上午 9:00 的時間設定好。然後依序完成三個步驟：建立一次性的每週五重複排程、確認之後每週都會自動沿用相同的內容來源、最後說明完成後不必每週重新建立同一套流程。這個 Skill 本身不能取代外部的寄送或排程系統，但要提供可以直接照做的設定步驟。

## Required behavior

- Do not ask the user to repeat the same weekly workflow.
- Do not invent names, dates, figures, events, recipient addresses, or scheduling details that the input does not provide.
- When the request includes a hard limit and a “keep everything” wish that conflict, keep the hard limit and say in one line what you left out.
- When a setting needed for the work is missing, choose the common default, name the exact value you chose, and finish the artifact.
- When outputting amounts or quantities that belong together, include their total.
- Make it clear in the output that the schedule can be reused after the first setup, so the same recipient, time, and content source can be applied every week without rebuilding the workflow.
- Deliver the finished artifact itself, not a plan or a request for access.

## Output format

Return the weekly report as ready-to-use email text with these parts:

- Subject
- To
- Send time
- Body

Body must contain the full message text in a paste-ready email format, including any bullet points or sectioned highlights from the user's input, not just a summary or memo.

Keep the body concise and usable. If the user provided multiple bullets or sections, preserve them as a single integrated report.

- If the user gives a specific time, show it exactly as provided.
- If the user only says Friday, the Send time field must still show every Friday and must not be omitted.

## If information is missing

Use clear labels such as “未提供” only for facts that are truly not given by the user. Do not guess.

## Reminder about action limits

You cannot send or schedule messages yourself. Prepare the content and say plainly that sending or scheduling is left to the person.