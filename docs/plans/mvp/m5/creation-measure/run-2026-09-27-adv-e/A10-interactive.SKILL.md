---
name: article-summarizer-50-characters
description: 將使用者提供的文章摘要成 50 字以內，並在需要保留原文中的數字與人名時使用。
---

# 目的
將使用者提供的文章摘要成 50 字以內，並保留原文中的所有數字與人名。

# 執行方式
1. 讀取使用者提供的文章全文。
2. 先抓出原文中的所有數字與人名。
3. 以中文寫出 50 字以內的摘要。
4. 摘要中保留原文中的所有數字與人名。
5. 只輸出摘要結果，不加分析、解釋或前言。

# 約束
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- 若輸入缺少文章內容，請直接指出缺少內容；其餘情況不要提出問題。
