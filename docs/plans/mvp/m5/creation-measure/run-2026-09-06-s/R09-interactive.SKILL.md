---
name: resume-to-data-analyst
description: Rewrite a provided resume into a one-page Chinese version tailored to data analysis roles. Use this skill when the user supplies resume text and wants a direct, copy-ready rewrite without inventing facts.
---

# Resume to Data Analyst

Use this skill when the user provides resume content and wants it rewritten into a one-page Chinese version tailored to data analysis roles.

Follow these rules exactly:
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Do not invent experience, education, skills, tools, metrics, employers, projects, or achievements.
- Keep the result copy-ready and concise enough for one page.
- Emphasize information that is relevant to data analysis, such as data handling, reporting, SQL, Excel, R, visualization, dashboards, or related coursework only when those items are present in the input.
- If the input omits a detail needed for a polished resume, preserve that omission or mark it as 'not given' rather than filling it in.
- Output only the rewritten resume text unless the user explicitly asks for commentary.

## Procedure
1. Read the resume exactly as provided.
2. Identify the facts already present: education, work experience, projects, tools, and outcomes.
3. Reorder and rephrase those facts to foreground data-analysis relevance.
4. Remove or de-emphasize material that does not support the target role, but do not add new facts.
5. Ensure the final text stays compact and directly usable as a one-page resume.
6. If a section title or detail is not given in the source, do not fabricate it; write 'not given' only when needed.

## Output expectations
- Produce a polished resume rewrite in Chinese.
- Keep the tone professional and role-focused.
- Preserve factual accuracy strictly.
- Return only the final resume content.