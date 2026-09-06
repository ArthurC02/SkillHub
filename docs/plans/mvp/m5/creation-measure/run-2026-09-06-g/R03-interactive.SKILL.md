---
name: news-headline-digest-emailer
description: When you need a daily morning digest from three news sites, this skill gathers their headlines, condenses them into five Chinese summaries, and prepares an email-ready result for the recipient.
---

# News Headline Digest Emailer

## What this skill does
This skill turns headlines from exactly three news sources into five concise Chinese summaries and prepares the content to be sent by email every morning.

Use it when the user asks for a recurring morning news digest, a summary from multiple news sites, or an email-ready brief based on current headlines.

## Inputs you must use
- Three news site URLs or source identifiers.
- The recipient email address.
- Any explicit summarization rules provided by the user.
- Any scheduling requirement stated by the user, especially the morning delivery time.

If any of those are missing, ask for the missing information before producing the digest.

## Workflow
1. Read the three supplied news sources.
2. Extract the current headline set from each source.
3. Select the most relevant headline information across the three sites.
4. Write exactly five Chinese summaries.
5. Keep the summaries concise, news-like, and directly usable in an email body.
6. Format the result so it can be copied into an email or sent by an email tool if available.
7. If the user asked for daily morning delivery, preserve that schedule intent in the output notes.

## Output requirements
Return a five-item summary list that is directly usable as email body content.

## Writing rules
- Produce exactly five summaries.
- Write the summaries in Chinese.
- Base the summaries on the headlines from the three news sites.
- Do not add extra summaries beyond the five requested.
- Do not invent news that is not supported by the source headlines.
- Keep the output directly usable as email content.

## If a source cannot be read
If one of the three sources is unavailable, report that source as unavailable and continue only if the remaining information still supports the requested digest. Do not guess missing headlines.

## If email delivery is not available
Prepare the email-ready text and clearly label it for manual sending. Do not claim that a message was sent unless an email tool actually performed the delivery.