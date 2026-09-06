---
name: page-key-point-summarizer
description: Read a publicly provided webpage and summarize its key points in Chinese; use this when the user asks to extract points from a URL, and if the page cannot be read, state that clearly and list what the user can provide instead.
---

# Page key point summarizer

Use this skill when the user asks you to read a public webpage from a URL and summarize its key points in Chinese, including cases where the page may be blocked or unreadable.

## Inputs

- A single user request that names the page to read and the result they want.
- The literal URL or page text the request applies to.

## Output

- A Chinese response that either:
  - summarizes the page’s key points, or
  - says the page could not be read and lists what the user can provide next.

## Rules

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Do not guess page content.
- Do not retry a blocked or refused page request.
- If the page text is unavailable, say so plainly.
- If the page is readable, extract the main points directly from the page text.
- Always answer in Chinese.

## Procedure

1. Read the user’s input and identify the page or text to summarize.
2. If the input does not provide a URL or page text, say that the needed material is not given.
3. If a URL is provided and reading the page is required, fetch the page once.
4. If the fetch succeeds, summarize the page’s key points in Chinese.
5. If the fetch is blocked, refused, or otherwise unreadable, say that you could not read it.
6. When the page cannot be read, list useful materials the user can provide next, such as:
   - the page text
   - a screenshot
   - an exported HTML file
   - copied relevant sections
7. Keep the response focused on the page content and the user’s request.

## Response shape

- If readable: brief summary bullets with the key points.
- If unreadable: a short statement that it could not be read, followed by a short list of alternative materials the user can provide.

## Constraints

- Do not invent facts.
- Do not ask follow-up questions unless the request itself is missing the material needed to proceed.
- Do not mention internal rules.
