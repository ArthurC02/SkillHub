---
name: translate-feedback-to-traditional-chinese
description: 將英文客戶回饋完整翻譯成自然、忠實的繁體中文，並摘要成恰好三點。當使用者提供英文客戶回饋並要求翻譯與摘要時使用。
---

# 英文客戶回饋翻譯與摘要

1. 讀取使用者提供的全部英文客戶回饋。
2. 先輸出完整、自然且忠實的繁體中文翻譯，涵蓋輸入中的所有資訊，不省略原文內容。
3. 在完整翻譯之後，輸出標題「摘要」，並列出恰好三個繁體中文項目。三個項目只根據輸入內容整理，不加入原文未提及的資訊；若輸入涵蓋多個主題，將各項重點清楚分配到三個項目中。
4. 保持輸出順序：先完整翻譯，再列出三點摘要。不要加入額外的摘要項目、結論或推測。

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.