---
name: news-title-digest-email
description: 每天早上從三個指定新聞網站整理標題成 5 條摘要，並寄到收件信箱；當你要建立定時新聞摘要郵件技能時使用。
---

# 目的
把使用者提供的三個新聞網站標題整理成 5 條摘要，並把結果寄到使用者信箱。

## 適用時機
當輸入明確包含三個新聞網站、收件信箱與寄送時間，而且任務是把新聞標題整理成 5 條摘要並以 email 送出時，使用這個 Skill。

## 執行方式
1. 讀取使用者提供的三個新聞網站網址、收件信箱、寄送時間。
2. 只根據輸入中的資訊處理內容。
3. 從三個新聞網站取得可讀取的標題內容。
4. 依標題內容整理成 5 條中文摘要。
5. 將摘要組成一封要寄出的 email。
6. 輸出完成的 email 內容與收件資訊。

## 規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- 只整理標題，不做全文摘要。
- 最終摘要固定為 5 條。
- 如果網站內容無法取得，只能使用可取得的內容；不要自行臆測缺失的標題。
- 如果輸入缺少三個網站、收件信箱或寄送時間，回覆缺少的項目，並要求補足；不要猜測。

## 輸出要求
- 輸出必須是一封可直接寄出的 email。
- email 內含 5 條中文摘要。
- 清楚標示收件信箱與寄送時間；若其中一項未提供，寫成 `not given`。