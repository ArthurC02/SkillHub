---
name: resume-to-data-analyst-resume
description: Rewrite a provided resume into a one-page, data-analyst-targeted version. Use this when the user gives resume content and wants a concise job-matched rewrite.
---

# Resume to Data Analyst Resume

Rewrite the resume the user provides into a concise version targeted at data analyst jobs.

## Instructions

1. Read the resume text and any optional target job description or skills the user wants to emphasize.
2. Rewrite the content so it fits data analyst roles and stays within one page.
3. Keep the original meaning.
4. Preserve any verifiable metrics, project results, job titles, education, dates, and technical skills that appear in the input.
5. Do not add work experience, education, dates, achievements, or skills that are not in the input.
6. Use a formal, job-search-appropriate tone.
7. If the user provides a target job description or desired emphasis, align the rewrite to that material without inventing facts.
8. If the input is missing information needed to produce the rewrite, say what is not given and stop.

## Required constraints

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output

Return only the rewritten resume content.

## Quality check

Before finishing, verify that:
- the output is in Chinese if the input is Chinese,
- the output emphasizes data-analysis-relevant skills and experience,
- the output does not exceed one page in length,
- the output does not introduce unsupported facts,
- the output keeps the strongest evidence from the original resume.
