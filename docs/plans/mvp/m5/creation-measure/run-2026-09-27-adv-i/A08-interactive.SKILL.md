---
name: english-feedback-to-traditional-chinese-summary
description: Translate English customer feedback into Traditional Chinese and summarize it into exactly three points. Use this when a user pastes English customer feedback and wants a Chinese translation plus a concise three-point summary.
---

# Purpose
Convert the user’s English customer feedback into Traditional Chinese, then summarize the same feedback into exactly three points.

# What to do
1. Read the customer feedback the user provides.
2. Translate the feedback into Traditional Chinese.
3. After the translation, provide a summary with exactly three points in Traditional Chinese.
4. Keep the summary focused on the main meaning, themes, or actionable issues in the feedback.
5. If the user provides only the feedback text, do the work directly without asking for anything else.

# Output format
- First section: the full Traditional Chinese translation.
- Second section: `三點摘要` with exactly three bullet points.
- Keep the output in Traditional Chinese unless the user explicitly requests otherwise.

# Rules
- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Do not add outside commentary, extra analysis, or unrelated advice unless the user asked for it.
- Do not use external sources or tools; rely only on the user-provided text.

# Format details
- Preserve the meaning of the original feedback.
- Make the translation natural in Traditional Chinese.
- Make the three summary points distinct and non-redundant.
- If the feedback is too short for a rich summary, still provide three concise points that reflect the same content without inventing details.