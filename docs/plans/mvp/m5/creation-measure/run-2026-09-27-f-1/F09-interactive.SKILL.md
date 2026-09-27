---
name: gallery-opening-invitation-writer
description: Writes formal, elegant gallery opening invitation copy from provided exhibition details. Use it when you need an invitation that must preserve the exhibition title, date range, opening reception time, and venue exactly as given.
---

# What this Skill does
Write a formal, elegant gallery opening invitation from the exhibition details the user provides. Preserve these facts exactly as given: exhibition title, exhibition period (start and end dates), opening reception time, and venue.

# When to use it
Use this Skill when the user asks for a gallery opening invitation, opening notice, or invitation copy that needs a polished tone while keeping the core event facts unchanged.

# Steps
1. Read the user's input and identify the four required facts:
   - exhibition title
   - exhibition period
   - opening reception time
   - venue
2. If all four facts are present, draft the invitation in a formal, elegant tone.
3. Keep the four facts verbatim as much as the input allows; do not change numbers, dates, time, or place names.
4. If any of the four facts are missing, mark each missing fact as「未提供」in the output language and keep writing only with the facts that were given.
5. If the input contains contradictory event facts, do not choose between them; state that the input is contradictory and ask the user to confirm the correct details.
6. Output only the invitation text, with no analysis or checklist.

# Writing rules
- Use respectful, polished language.
- Keep the tone ceremonial but concise.
- Do not add false details such as extra dates, locations, or hosts.
- Do not infer facts that were not provided.
- If the user supplies a desired length or format, follow it as long as it does not conflict with the required facts.
- If the user language is Chinese, write in Chinese.