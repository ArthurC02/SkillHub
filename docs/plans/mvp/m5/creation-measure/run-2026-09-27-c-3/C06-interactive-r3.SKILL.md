---
name: weekly-report-auto-send
description: Create a reusable Skill for preparing and scheduling a weekly Friday report email to a manager. Use this when the user wants a repeatable workflow that separates report drafting from scheduled sending and may need a common default for the mail or scheduling service.
---

# Weekly Report Auto-Send Skill

## Purpose
Help the user prepare a weekly report email and set up a recurring Friday schedule for sending it to a manager.

Use this Skill when the request is about:
- sending a weekly report on Fridays,
- separating report writing from scheduled sending,
- or creating a reusable workflow for recurring status updates.

## Core task
Produce the final weekly report content and the scheduling instructions in one pass.

## Required user inputs
- Ask the user to provide the manager’s email address and the weekly report content for each run.
- Treat both items as required inputs; if either one is missing, state that it is missing instead of assuming it exists.
- Do not treat the sample values as defaults for future runs.

## Working assumptions when details are missing
- If the mail or scheduling service is not specified, use the common default approach: send email through a typical mail service and schedule it as a weekly recurring Friday job.
- Name the assumption you chose in the output.
- If the manager’s email address is missing, mark it as not given.
- If the report content is missing, write a usable weekly-report template instead of stopping.
- If the needed sending or scheduling system is unavailable, prepare the content ready to use and say plainly that sending or scheduling is left to the person.

## Instructions
1. Identify the manager recipient and the weekly report content from the input.
2. If the recipient or report content is missing, fill the gap with the common default behavior described above and state the assumption used.
3. Draft the weekly report in clear business language.
4. Separate the work into two parts:
   - report content,
   - scheduling instructions for every Friday.
5. Include the final recipient address, if given.
6. Include the timing as every Friday.
7. If the request asks to post, send, schedule, monitor, or fetch something, do not claim you performed it. Return the content ready to use and say that sending or scheduling is left to the person.
8. Do not invent facts the input does not give. Any missing name, date, figure, or event must be marked as not given in the language of the output.
9. When the output lists amounts or quantities that belong together, give their total.
10. Finish the work instead of asking follow-up questions once a default can be used.

## Output format
Return the result with these sections:
- Assumptions
- Recipient
- Weekly report content
- Schedule
- Ready-to-use message

## Content rules
- Keep the report concise and professional.
- Preserve any facts, figures, and totals provided by the user.
- If several quantities are present, include their total when relevant.
- If the user gives no subject line, use a simple default subject such as "Weekly Report".
- If the user gives no schedule tool, write a generic weekly Friday schedule instruction.

## Ready-to-use message
Provide the final email text exactly as the person could paste or send it.

## Limitations
- This Skill does not actually send email or create calendar events.
- It prepares the content and the schedule instructions only.
- The person is responsible for connecting the mail and scheduling service.