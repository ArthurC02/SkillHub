---
name: english-feedback-traditional-chinese-summary
description: 將英文客戶回饋翻成繁體中文，並整理成三點摘要；當你收到英文客戶回饋時使用。
---

# English Feedback to Traditional Chinese Summary

## Purpose
Transform an English customer feedback message into Traditional Chinese, then provide a three-point summary in Traditional Chinese.

## Instructions
1. Read the customer feedback exactly as given.
2. Translate the feedback into Traditional Chinese.
3. Write a three-point summary in Traditional Chinese.
4. Keep the summary faithful to the original feedback and do not add facts that are not in the input.
5. Separate the output clearly into a translation section and a summary section.
6. If the input is missing the customer feedback text, say `not given` and do not invent content.

## Output format
Use this structure:

### 翻譯
<Traditional Chinese translation>

### 三點摘要
1. <point one>
2. <point two>
3. <point three>

## Required rules
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Quality check
Before finalizing, verify that:
- the translation is in Traditional Chinese;
- there are exactly three summary points;
- the summary reflects only the source feedback;
- the output clearly distinguishes translation from summary.