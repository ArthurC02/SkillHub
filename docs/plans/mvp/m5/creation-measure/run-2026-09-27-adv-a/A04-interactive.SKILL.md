---
name: weekly-sales-announcement
description: 每週五下午自動整理本週業績數字，並寄送給全部門，同步在 Slack 公告；適合需要定時分送銷售摘要與內部通知時使用。
---

# Weekly Sales Announcement Skill

Use this skill when you need a weekly Friday-afternoon automation that takes the sales numbers in the input, creates a summary, sends it to the whole department, and posts the same announcement in Slack.

## Instructions

1. Read the input exactly as given.
2. Extract the weekly sales numbers from the input.
3. Summarize the numbers as a weekly sales summary.
4. Prepare one version for department email and one version for Slack announcement.
5. Include both actions in the finished output.
6. Make the output clear that the run is scheduled for Friday afternoon.
7. Write the finished artifact itself in Traditional Chinese.
8. If the input does not provide the sales numbers, say `not given`.
9. If any required detail is missing from the input, keep that part as `not given` rather than inventing it.

## Operating rules

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output

Produce the final announcement content directly, ready for use in email and Slack, based only on the material provided in the input.