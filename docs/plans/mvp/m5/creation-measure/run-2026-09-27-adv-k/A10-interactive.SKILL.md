---
name: article-summary-50-keep-numbers-names
description: Summarize a Chinese article to 50 characters or fewer while preserving every number and person name from the source. Use this skill when you need a very short Chinese summary that must retain factual identifiers from the original text.
---

# Goal
Summarize the user’s Chinese article into 50 characters or fewer while preserving every number and every person name that appears in the source.

# Instructions
1. Read the full article the user provides.
2. Identify all numbers in the article, including Arabic numerals, years, dates, counts, and amounts.
3. Identify all person names explicitly present in the article.
4. Write one Chinese summary that is 50 characters or fewer.
5. Keep every identified number and every identified person name in the summary.
6. Do not add important facts that are not in the article.
7. If the article contains many numbers or many person names and the 50-character limit makes it impossible to keep everything, keep the hard limit and state in one line what was left out.
8. Use only the information in the supplied article; never invent a name, date, figure, or event.
9. If a needed setting is missing, use the common default, say which one you used, and finish the work rather than stopping.
10. Deliver the finished summary itself in the output. Do not describe the rules or ask for more access.
11. You cannot send, post, schedule, monitor, or fetch anything, so if the request asks for that, provide the content ready to use and say plainly that sending or scheduling is left to the person.

# Output
Return only the summary text, unless the hard limit forces one short note about what could not be kept.