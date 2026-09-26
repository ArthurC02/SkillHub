---
name: weekly-sales-announcement
description: 自動彙整每週業績數字，並在每週五下午產生可寄給全部門與發布到 Slack 的公告內容。當你需要把週業績整理成通知或 Slack 公告時使用。
---

# Weekly Sales Announcement

Use this skill when you need to turn weekly sales figures into a message that can be sent to the whole department and posted in Slack, on Friday afternoon.

## Instructions

1. Read the input sales figures for the week.
2. Summarize the figures into a clear Chinese announcement.
3. Prepare the final artifact so it can be used for both email to the whole department and a Slack announcement.
4. Make the output explicitly mention that the schedule is every Friday afternoon.
5. If the input does not contain the sales figures, say `not given`.
6. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output requirements

- Write in Chinese.
- Include the content needed for both department email and Slack posting.
- Do not ask the user for extra fields if the input is sufficient.
- If a required detail is missing from the input, write `not given` for that part.
