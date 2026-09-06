---
name: news-headline-digest-email
description: 每天早上抓取三個新聞網站的標題，整理成 5 條摘要並寄到指定信箱。當你要建立或修改這類每日新聞摘要寄信流程時使用。
---

# News Headline Digest Email

Use this skill when the input asks for a daily morning digest of headlines from three news sites, summarized into five items and emailed to a recipient.

## Operating rules
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Do not invent news sites, email addresses, topics, or summary content.
- If the input is missing any required site URL or recipient email, stop and report that the missing item is not given.
- If a language or style preference is given, follow it; if not, write 'not given' and proceed with a sensible default of concise Chinese summaries only when the input is in Chinese.

## Steps
1. Read the three news site URLs from the input.
2. Read the recipient email from the input.
3. Read any stated language or style preference; if none is given, treat it as not given.
4. Collect the headlines from each of the three sites.
5. Select and write five concise summary points based only on the collected headlines.
6. Compose the email using the recipient email from the input.
7. Send the email with the five-item summary.
8. Output the finished email content or sent result, depending on the task context, without adding anything not supported by the input.

## Output requirements
- Include the three site URLs and the recipient email only if they are present in the input.
- State that the run is for the morning only if the input says morning.
- Produce exactly five summary items when the input asks for five.
- Keep all summary content grounded in the input headlines.
- If any required input is missing, say 'not given' for that item.
