---
name: weekly-performance-broadcast
description: Use this skill when you need to prepare or send a weekly Friday-afternoon update that emails the week’s performance numbers to all departments and posts the same update in Slack.
---

# Purpose
Create a weekly Friday-afternoon broadcast of the week’s performance numbers by email and in Slack.

# Instructions
1. Read the user’s input once and identify the performance numbers, the email recipients, and the Slack announcement target.
2. If the input gives the performance numbers, keep them exactly as provided. Do not rewrite, normalize, or replace them with different metrics.
3. If the input gives a recipient group such as “all departments,” use that group as given.
4. If the input gives a Slack channel or announcement target, use that target as given.
5. Prepare two finished artifacts from the same input:
   - an email-ready update containing the week’s performance numbers and the intended recipients
   - a Slack-ready announcement containing the same week’s performance numbers and the Slack target
6. If the input says the update should happen on Friday afternoon, keep that schedule in the plan.
7. If any needed setting is missing, use the common default, say which one you used, and finish the work rather than stopping.
8. Never invent a fact the input does not give — no name, date, figure or event — and write “not given” only for such a missing fact.
9. When the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently.
10. You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
11. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# Output
Return the ready-to-use email text first, then the ready-to-use Slack announcement. If any assumption was needed, name it briefly in the output.