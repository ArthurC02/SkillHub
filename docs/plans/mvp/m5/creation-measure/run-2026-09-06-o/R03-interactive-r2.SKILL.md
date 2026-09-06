---
name: news-headline-digest-emailer
description: 當使用者要每天早上把三個新聞網站的標題整理成 5 條摘要並寄到信箱時使用。這個 Skill 會把來源、摘要與寄信需求整理成可實作的規格。
---

# Purpose
Create a Skill specification for a daily morning workflow that collects headlines from three news websites, condenses them into 5 summaries, and emails them to the user's inbox.

# Instructions for the agent
1. Read the user's request and extract the required task.
2. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
3. Directly output the specification text that can be submitted as a Skill; do not provide alternative solutions, do not ask for additional information, and do not merely restate the request.
4. Identify the three news website sources if they are present in the input; if they are not present, write 'not given'.
5. Identify the recipient email address if it is present in the input; if it is not present, write 'not given'.
6. Specify that the output must contain 5 summaries.
7. Specify that the workflow is for morning delivery and email delivery if the input states that.
8. If the input does not provide enough information to complete a required field in the Skill spec, write 'not given' rather than inventing it.
9. Produce a portable Agent Skill specification that can be used directly.

# Required content of the Skill specification
- The task to perform.
- The input needed to perform it.
- The output it must produce.
- The output must include exactly 5 summaries and the email delivery result.
- The scheduling requirement if stated in the input.
- The email delivery requirement if stated in the input.
- The three news website sources if stated in the input.
- The recipient email if stated in the input.

# Output shape
Return a clear Skill specification in Markdown with these sections:
- Purpose
- Inputs
- Outputs
- Operating constraints
- Missing information

# Notes
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.