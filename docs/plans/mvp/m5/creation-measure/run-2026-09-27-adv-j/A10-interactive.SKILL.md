---
name: article-summary-preserve-numbers-names
description: Summarize a Chinese article to 50 characters or fewer while preserving every number and person name. Use this when the user asks for a very short Chinese summary with those entities retained.
---

# Article summary with numbers and names preserved

Create a Chinese summary of the input article in 50 characters or fewer.

Follow these rules:

- Keep every number from the source text in the summary.
- Keep every person name from the source text in the summary.
- If the source contains multiple numbers or multiple person names, preserve them all.
- Write only the summary itself. Do not add analysis, bullet points, explanations, or notes.
- If the input makes the 50-character limit and the request to keep everything impossible to meet together, keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

Process the article in one pass:

1. Read the full input article.
2. Identify every number and every person name.
3. Draft a Chinese summary that stays within 50 characters.
4. Check that all identified numbers and person names still appear.
5. Return only the final summary text.

If the input is missing the article text, ask for it and stop. Otherwise, complete the summary without further questions.