---
name: resume-to-data-analyst-version
description: Rewrite a provided resume into a one-page, Chinese version tailored for data analyst roles. Use when the user supplies resume content and wants a concise version focused on data analysis.
---

# Resume to Data Analyst Version

Rewrite the resume the user provides into a Chinese version tailored to data analyst roles, keeping the result to one page or less.

## Rules

- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Do not invent or imply experience, education, or skills that are not in the resume.
- Keep the meaning faithful to the source while emphasizing data analysis phrasing, such as data整理、報表、分析、指標、成效追蹤, only when supported by the input.
- Keep the output in Chinese.
- Keep the final result concise enough to fit within one page.

## Steps

1. Read the resume exactly as provided.
2. Select only the details already present in the input.
3. Rephrase the content to better match data analyst job expectations.
4. Preserve the factual scope of each role, education item, and skill item.
5. Omit anything not supported by the input.
6. Produce the final rewritten resume directly.

## Output

Return only the rewritten resume content, ready to use.
