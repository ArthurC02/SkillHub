---
name: insurance-claim-result-notification-letter
description: Write a Chinese insurance claim result notification letter when given the claim number, approved amount, approval date, appeal channel, and appeal deadline. Use it to generate a concise notice that keeps each required item in its own sentence and stays within the 3-sentence limit.
---

# Insurance Claim Result Notification Letter

Use this skill to write a Chinese insurance claim result notification letter from the input you are given.

## What to do
1. Read the five required details from the user input:
   - 理賠案號
   - 核定金額
   - 核定日期
   - 不服核定的申訴管道
   - 申訴期限
2. Write the finished letter in Chinese.
3. Put each required detail in its own separate sentence, one detail per sentence.
4. Keep the whole letter to at most 3 sentences.
5. Do not combine two required details in the same sentence, even if you avoid commas or 頓號.
6. If all five details are present, produce the letter directly in one pass.
7. When all five details are present, compress the notice into at most three sentences by combining only non-required framing language; never merge two required details into the same sentence.

## If information is missing
- If any of the five required details is missing, write a usable template letter that clearly marks the missing item as「未提供」in Chinese.
- Do not ask follow-up questions.
- Still keep each required detail in its own sentence and keep the total to at most 3 sentences.

## Writing rules
- Use the exact input values when they are provided.
- Do not invent any claim number, amount, date, appeal channel, or appeal deadline that the input does not give.
- Keep the output limited to the letter text only.
- Do not explain the rules.
- Do not add bullet points, tables, or commentary.

## Default choices when needed
- If tone is not specified, use a neutral formal notice tone.
- If a formatting choice is not specified, use plain paragraphs.

## Output shape
- Return only the final Chinese letter.
- Make sure the five required items appear as separate sentences when all are provided.
- Make sure the total sentence count does not exceed 3.
- When all five details are provided, each required detail must still be in its own sentence. Keep the whole letter within 3 sentences by using one sentence for one required detail and placing any extra non-required framing only if it does not create a fourth sentence.