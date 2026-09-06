---
name: resume-to-data-analyst-rewrite
description: Rewrite a provided resume into a concise, Chinese version tailored for data analyst roles when the user supplies resume content and asks for a one-page rewrite. Use it to transform only the given material without inventing experience or skills.
---

# Resume to Data Analyst Rewrite

Use this skill when the user provides resume content and wants it rewritten for a data analyst role in Chinese, especially when they want the result kept to one page.

## Goal
Rewrite only the material the user supplied into a sharper resume that highlights analysis, reporting, data handling, Excel, SQL, and cross-functional coordination where those themes already exist in the source.

## Instructions
1. Read the resume carefully and identify all concrete facts already present.
2. Recast the experience so it sounds relevant to data analysis roles.
   - Emphasize data整理、報表、成效分析、資料彙整、流程優化、協作、簡報.
   - Keep job titles and employers accurate unless the user explicitly asks for a different format.
3. Do not invent tools, metrics, achievements, education, certifications, or responsibilities that are not in the source.
4. Keep the output concise enough to fit on one page.
   - Prefer a compact professional summary only if the source already supports it.
   - Trim low-value phrasing and remove repetition.
5. Preserve the original language as Chinese.
6. Return only the rewritten resume text unless the user asks for commentary.

## Writing approach
- Translate general business experience into data-adjacent language only when the source supports it.
- Convert duties into outcome-oriented bullets where possible without adding unsupported numbers.
- Keep contact details, education, and skills if they are present and relevant.
- If the source includes basic SQL, Excel, reporting, or data整理, surface those early.

## Quality check before finalizing
- Is the result in Chinese?
- Is it tailored toward data analyst roles?
- Is it one page or shorter in a typical resume layout?
- Does every claim come from the provided resume?

If any answer is no, revise the draft until all are yes.