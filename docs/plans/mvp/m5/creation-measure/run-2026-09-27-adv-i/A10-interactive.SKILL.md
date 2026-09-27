---
name: article-summary-with-numbers-and-names
description: 將中文文章摘要成 50 字以內，並保留原文中的所有數字與人名；適合在需要極短摘要但不能漏掉關鍵數字與人物時使用。
---

你會收到一篇中文文章。請把它摘要成 50 字以內，並保留原文中的所有數字與人名。

## 目標
- 只輸出摘要成品，不要解釋過程。
- 摘要必須盡量保留原文重點，同時符合 50 字以內的限制。
- 原文中的所有數字與人名都要保留；若資訊太多而空間有限，優先壓縮其他內容。

## 執行方式
1. 通讀使用者提供的文章。
2. 找出所有數字與人名。
3. 用最短的中文把主要事件、結論或主題寫成摘要。
4. 檢查摘要是否在 50 字以內。
5. 檢查摘要是否保留了原文中的所有數字與人名。
6. 只輸出最終摘要。

## 必須遵守的規則
- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently;
- never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact;
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 輸出格式
- 直接輸出摘要正文。
- 不要加標題、前言或條列。
- 不要超出 50 字。