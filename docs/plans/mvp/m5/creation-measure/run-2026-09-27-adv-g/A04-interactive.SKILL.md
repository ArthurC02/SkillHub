---
name: weekly-performance-announcement-draft
description: 將本週業績數字整理成可直接寄給全部門的週報與可直接貼到 Slack 的公告文案；當你需要為內部週五例行業績更新產出可用文字時使用。
---

# Weekly performance announcement draft

Use this skill when you need to turn a user-provided set of weekly performance figures into internal announcement copy for email and Slack.

## Instructions

1. Read the input exactly as given.
2. Extract the weekly performance figures, highlights, totals, growth rates, and any other provided notes.
3. Do not invent any fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
4. When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
5. You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
6. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
7. If the input does not include enough information to write a field, mark that field as 'not given' rather than guessing.
8. Produce two ready-to-use outputs:
   - one email-style message for the whole department
   - one Slack announcement
9. Keep the tone internal, concise, and suitable for company-wide sharing.
10. If multiple performance items are present, preserve all of them.
11. If totals, growth rates, or highlight notes are present, include them in both outputs.
12. Make it clear that the message refers to the current week’s performance update.
13. Use Traditional Chinese unless the input itself clearly uses another language.
14. End with a short note that sending or posting is left to the person.

## Output format

Return the content in this structure:

### 給全部門的週報信件
[ready-to-send email copy]

### Slack 公告
[ready-to-post Slack copy]

### 備註
[brief note that sending or posting is left to the person]
