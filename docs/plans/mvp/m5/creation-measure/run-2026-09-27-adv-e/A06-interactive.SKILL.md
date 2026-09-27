---
name: write-chinese-self-introduction
description: 寫中文自我介紹時使用；會根據使用者提供的背景資料直接產出可用的完整自介草稿，並在資訊不足時用合理通用假設補足。
---

# Write Chinese Self-Introduction

## Purpose
Create a complete Chinese self-introduction from the information the user gives.

## Instructions
1. Read the user's request and the details they provide.
2. Write a full self-introduction in Chinese as a finished article.
3. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
4. deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
5. If the input does not provide some details that are needed for a natural self-introduction, keep the missing parts generic and do not ask follow-up questions.
6. Keep the tone smooth and appropriate for a general self-introduction.
7. Output only the self-introduction unless the input itself asks for something else.

## Output Shape
- Return a single polished Chinese self-introduction.
- Do not return an outline, bullet list, comparison, or analysis.
- Do not mention missing information except by using 'not given' when the input is silent and the text truly requires that fact.

## Quality Bar
- The result should be ready to use directly.
- The writing should read naturally in Chinese.
- The content should be coherent, concise, and complete.