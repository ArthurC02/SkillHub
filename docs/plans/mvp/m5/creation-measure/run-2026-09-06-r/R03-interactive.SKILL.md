---
name: daily-news-summary-email
description: When you need a daily morning email that turns three specified news-site headlines into five Traditional Chinese summaries, use this skill to draft the ready-to-send message content.
---

# Daily News Summary Email

將三個新聞網站的標題整理成 5 條摘要，並輸出可直接寄出的電子郵件內容。

## 適用時機
當使用者提供三個新聞網站網址、收件信箱，並要求把每日早上的標題整理成 5 條摘要寄出時使用。

## 任務
根據輸入中的三個新聞網站與收件資訊，產生一封可直接寄送的繁體中文摘要郵件。

## 執行步驟
1. 讀取輸入中的三個新聞網站網址與收件信箱。
2. 擷取這三個網站的標題內容。
3. 從標題中整理出 5 條摘要。
4. 將摘要寫成完整的郵件內容。
5. 在輸出中保留來源網站資訊，讓每條摘要可追溯到其來源。
6. 以繁體中文輸出成品。

## 輸出要求
- 直接輸出可寄送的郵件內容。
- 內容包含 5 條摘要。
- 內容明確對應到 3 個來源網站。
- 內容使用繁體中文。
- 內容適合每日早上固定執行的自動化流程。

## 重要限制
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 當資訊不足時
如果輸入沒有提供三個網站網址或收件信箱，就在輸出中寫明缺少的項目，並使用 `not given` 表示未提供的資訊。