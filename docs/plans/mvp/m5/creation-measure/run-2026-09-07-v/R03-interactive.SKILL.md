---
name: news-title-summary-email
description: 從使用者提供的三個新聞網站標題整理成五條中文摘要並產生可寄送的郵件內容；在需要每日早上彙整新聞標題、輸出五條摘要郵件時使用。
---

# News Title Summary Email

當你收到一個要求，要把 3 個新聞網站的標題整理成 5 條摘要，並產生要寄出的電子郵件內容時，依下列步驟完成。

## 重要原則

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 步驟

1. 讀取輸入中提供的 3 個新聞網站網址、收件信箱與執行時間。
2. 如果網址、收件信箱或執行時間缺少任何一項，就在對應位置寫 `not given`。
3. 取得每個網站可直接看到的標題內容。
4. 只根據標題內容整理成 5 條中文摘要；不要加入標題以外的事實、評論或延伸解讀。
5. 在每條摘要後清楚標示來源網站。
6. 以繁體中文輸出可直接寄出的郵件內容。
7. 若某網站無法取得標題，就只使用已可取得的標題內容；不足的資訊寫 `not given`。

## 輸出要求

- 產出一封完整的郵件內容。
- 郵件內容必須包含 5 條摘要。
- 每條摘要都要對應新聞來源網站。
- 不得加入輸入未提供的資訊。
- 輸出語言為繁體中文。

## 使用工具

- 若輸入給了新聞網站網址，使用 `fetch_url` 讀取網站頁面內容。
- 只使用可直接取得的頁面內容，不做額外查證。