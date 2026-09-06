---
name: daily-news-title-summarizer-email
description: Use this skill when you need to turn titles from exactly three news sites into 5 Chinese summary points and format them as an email to a recipient each morning.
---

# Daily News Title Summarizer Email Skill

## Purpose
Turn the titles from exactly three news websites into 5 Chinese summary points and format the result as an email body ready to send each morning.

## Workflow
1. **Use the user-provided inputs as-is**: the three news website names or URLs, the recipient email address, and the morning send time.
2. **Fetch the titles from each of the three sites**.
   - If a site blocks access or the title source is unclear, report that as a limitation instead of inventing titles.
   - Do not use any source other than the three sites the user gave.
3. **Combine the collected titles into 5 Chinese summary points**.
   - Keep the summaries based on the titles, not on unrelated commentary.
   - Make the summaries concise and readable.
4. **Format the output as email body text**.
   - Include the recipient email address and the scheduled morning send time in the email-ready output.
   - Make the body directly usable as a message to send.
5. **Reflect all three source sites in the output**.
   - Ensure the result clearly shows that exactly three sources were used.
   - Do not add extra sources or omit any of the three.

## Output Requirements
- Produce 5 Chinese summary lines.
- Keep the result tied to newspaper or news-site titles only.
- Return the email body in a clean, sendable format.
- If the titles cannot be obtained from the provided sites, say so plainly and stop rather than guessing.

## Guardrails
- Do not ask the user for additional details unless the input does not include the three sites, the recipient email, or the send time.
- Do not invent website content.
- Do not claim the email has been sent unless sending is explicitly performed by the available environment.
- Do not use any source outside the three user-provided sites.