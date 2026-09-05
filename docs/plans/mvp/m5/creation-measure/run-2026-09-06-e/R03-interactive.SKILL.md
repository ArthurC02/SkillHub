---
name: news-headline-morning-digest
description: 每天早上從 3 個指定新聞網站擷取標題，整理成 5 條中文摘要並準備可寄送到使用者信箱的郵件內容；適合需要固定接收新聞重點整理時使用。
---

# news-headline-morning-digest

## What this skill does
This skill collects headlines from exactly three user-provided news websites, condenses them into five Chinese summary bullets, and prepares an email-ready message for the user's inbox.

## When to use it
Use this skill when the user wants a daily morning digest of news titles from three specified sites, summarized into five points and sent by email.

## Inputs required
- Three news website URLs provided by the user.
- The recipient email address.
- The desired send time or a clear indication that the task should run every morning.

If any of these are missing, ask for the missing information before proceeding.

## Procedure
1. Verify that exactly three news website URLs are provided.
2. Verify that a recipient email address is provided.
3. Fetch the latest headlines from each site.
4. Select the most relevant headlines across the three sites.
5. Write five concise Chinese summary bullets based only on the collected headlines.
6. Prepare an email body that includes:
   - a short greeting or subject line context,
   - the five summary bullets,
   - the source websites or headline references used.
7. Check that no source URL is invented or replaced with an unprovided URL.
8. Output the email-ready content in a clean, copyable format.

## Output format
Return the digest as plain text with these sections:
- Subject
- Greeting or opening line
- Five summary bullets
- Source list
- Closing line

## Quality rules
- Use only the provided websites as sources.
- Do not fabricate headlines.
- Keep the five summaries aligned with the actual headline content.
- If the sites provide too few usable headlines, say so clearly and summarize only what is supported by the sources.
- Do not include unrelated commentary.

## Error handling
If the user does not provide all three URLs or the email address, respond with a short request for the missing items.
If a site cannot be accessed, note which site failed and continue only with the sources that are accessible, unless too little information remains to produce five supported summaries.

## Notes
This skill prepares email content; actual delivery depends on the surrounding environment's mail capability.