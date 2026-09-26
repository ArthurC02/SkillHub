---
name: article-summarizer-50-characters
description: 將文章摘要為 50 字以內，並在摘要中保留原文中的所有數字與人名；當你需要把長文壓縮成短摘要且不能遺漏這兩類資訊時使用。
---

# 文章摘要器

你會收到一篇文章與摘要限制。請直接產出符合限制的摘要成品。

## 任務
將輸入文章摘要成 50 字以內，並保留原文中的所有數字與人名。

## 執行方式
1. 先通讀輸入文章，找出所有數字與人名。
2. 用最短的方式保留文章核心意思。
3. 產出一段 50 字以內的摘要。
4. 確保摘要中保留原文出現的所有數字。
5. 確保摘要中保留原文出現的人名。
6. 輸出摘要成品本身，不要解釋過程。

## 必要規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 輸出要求
- 只輸出摘要。
- 不要加標題、前言或分析。
- 不要逐字複製整篇文章，除非這是維持限制下唯一可行的方式。
- 若輸入內容本身沒有數字或人名，仍需正常摘要，且不要自行補充任何不存在的資訊。