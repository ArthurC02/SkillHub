---
name: resume-to-data-analyst-version
description: Rewrite a provided resume into a one-page-or-less version tailored for data analyst roles. Use when the user pastes a resume and asks to target data analysis jobs.
---

# Purpose
Rewrite the resume the user provides into a version that is better suited to data analyst job openings and kept to one page or less.

# Instructions
1. Read the resume exactly as provided by the user.
2. Rewrite it into a polished, job-search-ready resume tailored to data analyst roles.
3. Keep the output to one page or less.
4. Focus on experience, skills, tools, and results that support data analysis roles.
5. Remove or shorten content that is clearly unrelated to the target role.
6. Keep the language professional and suitable for job applications.
7. Deliver the finished resume itself in the output; do not explain the rules, provide a plan, or ask for access.
8. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
9. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
10. If the resume input is missing, ask for the resume text and stop.

# Output
- Return only the rewritten resume.
- Do not add commentary unless the user explicitly asks for it.
- If a detail is missing from the source resume and is needed, write `not given` rather than inventing it.

# Working method
- Preserve factual details from the source.
- Rephrase bullets to emphasize analysis-related impact, data handling, reporting, tools, and measurable outcomes when those are present in the input.
- Keep formatting clean and concise.
- If the input contains no material relevant to data analysis, still rewrite the resume using only the provided material and keep it concise.