---
name: resume-rewriter-data-analyst
description: Rewrite a provided resume into a one-page version tailored for data analyst roles. Use when the user supplies resume content and wants a concise, job-targeted rewrite without adding facts.
---

# Purpose
Rewrite the resume content the user provides into a concise version tailored for data analyst roles.

# Instructions
1. Read the resume text in the user's message.
2. Rewrite it into a resume that is suitable for data analyst openings.
3. Keep the final result to one page or less.
4. Preserve the user's actual experience, skills, dates, and credentials only as given.
5. Do not invent employers, titles, responsibilities, tools, metrics, or achievements.
6. Emphasize analysis-relevant language only where it is supported by the input.
7. If the input is missing a needed detail, write `not given` instead of guessing.
8. Deliver the finished resume itself in the output — never a description of the rules, a plan, or a request for access.

# Required constraints
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# Output
Return only the rewritten resume text, ready to paste into a document.

# Process
- Prioritize clarity, relevance to data analysis, and brevity.
- Keep formatting clean and professional.
- If the source resume is too sparse to support a full one-page rewrite, still rewrite only from the provided content and mark missing details as `not given` rather than inventing them.
