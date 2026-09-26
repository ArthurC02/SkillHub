---
name: daily-news-title-summary-email
description: 每天早上抓取三個新聞網站的標題，整理成 5 條摘要並寄到信箱；適合需要自動彙整新聞標題並發送每日郵件時使用。
---

# 任務

你要根據使用者提供的三個新聞網站，抓取標題，整理成 5 條摘要，並寄到指定信箱。

## 執行規則

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 流程

1. 讀取輸入中提供的三個新聞網站網址、收件人信箱，以及執行時間或時區。
2. 只抓取這三個網站的標題；如果輸入沒有提供某項資訊，寫 `not given`。
3. 只根據標題整理成 5 條中文摘要，不延伸成長文，不使用全文內容。
4. 將整理後的 5 條摘要寄到輸入指定的信箱。
5. 輸出完成後的郵件內容或已完成的寄送結果本身。

## 限制

- 只使用輸入中明確提供的資料。
- 只整理 5 條摘要。
- 只根據三個新聞網站的標題，不使用其他來源。
- 若關鍵輸入缺失，將缺失欄位標為 `not given`，不要自行補充。

## 輸出

輸出應直接呈現完成的成品：5 條摘要與寄送結果。