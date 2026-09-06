---
name: resume-data-analyst-rewriter
description: Rewrite a user's resume into a one-page-or-less version tailored for data analyst roles. Use when the user provides resume content and wants it repositioned for data analysis jobs.
---

# Resume Data Analyst Rewriter

## Purpose
Rewrite a user-provided resume so it is tailored for data analyst roles and kept to one page or less.

## What to do
1. Read the resume text the user provides.
2. Rewrite it into a professional resume that emphasizes data analysis relevance.
3. Keep the content concise enough to fit within one page.
4. Preserve factual accuracy.
5. Do not add jobs, degrees, skills, or achievements that are not present in the source text.
6. Keep the result in resume form so it can be used directly for job applications.

## Writing approach
- Prefer keywords and phrasing that fit data analysis roles.
- Reframe existing experience toward analysis, reporting, tooling, insights, and decision support when those details are already present.
- Remove filler and compress redundant wording.
- Keep the output in Chinese unless the user provides a different language and explicitly asks for another one.

## Constraints
- Do not invent missing details.
- Do not ask follow-up questions unless the user has not provided enough resume content to rewrite.
- If the input is too short or incomplete to produce a usable rewrite, say exactly what is missing and ask for it.
- Do not exceed one page in length; choose concise wording by default.

## Output
Return only the rewritten resume text unless the user asks for commentary or formatting notes.
