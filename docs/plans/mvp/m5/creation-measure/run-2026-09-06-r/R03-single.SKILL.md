---
name: daily-news-summary-email
description: Use when you need a daily workflow that gathers headlines from three news sites, condenses them into five concise summaries, and sends them by email. This skill covers planning and drafting the process, but it cannot itself browse the live web or send email without external tools and configured access.
---

# Daily news summary email

## What this skill does
Create a daily routine that collects headlines from three news websites, distills them into five short summary bullets, and prepares an email to the recipient.

## Important limitation
This task requires live web access and email delivery. If the agent does not have browsing and email-sending tools or configured credentials, it cannot complete the task end-to-end. In that case, gather the source URLs, the recipient address, and the preferred send time, then prepare the summary text and email draft for a human or another tool to send.

## Procedure
1. **Confirm the sources**
   - Identify the three news websites to use.
   - Prefer stable homepage, section, or RSS URLs.
   - If the user did not specify the sites, ask for them before proceeding.

2. **Confirm the delivery details**
   - Get the recipient email address.
   - Confirm the send time and timezone for “every morning.”
   - Confirm whether weekends should be included.

3. **Collect headlines**
   - Visit each of the three sites.
   - Extract the current top headlines or the most relevant headlines from the chosen section.
   - Keep a short note of the source site for each headline.

4. **Deduplicate and select**
   - Remove repeated or near-duplicate stories across the three sites.
   - Choose the five most important or representative items.
   - If fewer than five distinct stories are available, use all distinct stories and note that the list is shorter.

5. **Write the five summaries**
   - Summaries should be concise, one bullet each.
   - Each summary should state the core event and, if useful, the significance.
   - Avoid speculation and avoid adding facts not present in the headlines or article snippets.
   - Keep the tone neutral and factual.

6. **Prepare the email**
   - Subject: a clear daily-news subject, such as `Daily News Summary`.
   - Body structure:
     - brief greeting
     - one-line note that the following are the day’s top items
     - five numbered or bulleted summaries
     - optional source list at the end
   - Include the source site names for traceability.

7. **Send or stage the email**
   - If email-sending tools and credentials are available, send the message.
   - If not, output the final email text exactly as it should be sent and state that sending requires configured email access.

8. **Quality check**
   - Verify the email contains exactly five summary items unless there are fewer distinct stories.
   - Verify the summaries are based on the collected headlines.
   - Verify the recipient, subject, and send time are correct.

## Suggested operating pattern for a daily run
- Morning trigger fires.
- Fetch headlines from the three sources.
- Produce five concise summaries.
- Send the email immediately or queue it for the configured morning time.

## If information is missing
Ask for:
- the three news website URLs
- the recipient email address
- the preferred morning send time and timezone
- whether weekends are included
- whether the user wants only headlines or headline-plus-brief context

## If tools are unavailable
State plainly that live browsing and email delivery are required and cannot be completed inside this skill alone. Provide the exact checklist of information needed and a draft email body based on any headlines the user supplies.
