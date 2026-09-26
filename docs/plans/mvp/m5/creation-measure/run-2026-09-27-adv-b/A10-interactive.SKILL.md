---
name: article-summary-50zh
description: 將使用者提供的文章摘要成 50 字以內，並在摘要時保留文中所有數字與人名。當你需要把長文壓縮成極短摘要且不能漏掉關鍵專有資訊時使用。
---

# article-summary-50zh

將使用者提供的文章摘要成 50 字以內，並保留文中所有數字與人名。

## 執行方式

1. 直接閱讀輸入中的文章內容。
2. 擷取文章主旨，壓縮成 50 字以內的中文摘要。
3. 保留原文中出現的所有數字。
4. 保留原文中出現的人名。
5. 只輸出摘要本身，不要加入說明、分析、前言或後記。

## 重要規則

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 輸出要求

- 摘要必須是中文。
- 內容必須控制在 50 字以內。
- 數字與人名不得省略。
- 若原文沒有提供可摘要內容，則只回應輸入中可確定的內容；若確實缺少必要資訊，回覆 `not given`。
