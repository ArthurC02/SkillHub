---
name: news-title-digest-mailer
description: When you need a daily morning summary of headlines from three news sites delivered by email, use this skill to collect the titles, condense them into five short summaries, and draft or send the email.
---

# Goal
Turn the headlines from three news websites into five short summaries and deliver them by email.

## Use this skill when
- A user gives three news-site sources and wants a morning summary by email.
- The user wants five headline-based summaries rather than a full news report.

## Instructions
1. Read the user input and extract the three news websites, the recipient email address, and the requested morning timing if provided.
2. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
3. Use only the sources the user provided. Do not introduce additional websites, extra feeds, or outside topics.
4. Gather the latest headlines from each of the three sites.
5. Select headline material that can support five concise summaries.
6. Write exactly five summaries. Keep them tied to the headlines and avoid inventing details not supported by the input.
7. Directly output a sendable email draft; do not only describe the process, rules, or ask for more information.
8. Include the recipient email address in the output when it is given; if it is not given, write 'not given'.
9. If the requested morning time is given, include it in the output; if it is not given, write 'not given'.
10. If the user has not provided enough information to identify the three news sites or the recipient email, say what is missing and stop.

## Output
Return the email draft or sent-message content as the finished artifact.
- Subject: a short morning news digest subject.
- Body: five concise summaries based on the provided headlines.
- Recipient: the email address from the input, or 'not given' if absent.
- Timing: the morning schedule from the input, or 'not given' if absent.

## Constraints
- Use only the input provided.
- Do not add unsupported facts.
- Keep the result to five summaries.
- Deliver the finished artifact itself in the output.