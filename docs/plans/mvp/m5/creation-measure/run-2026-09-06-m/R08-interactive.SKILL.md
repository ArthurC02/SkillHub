---
name: return-request-reply-letter
description: 收到客戶退貨申請時，用來判斷是否在 7 天內，並輸出對應的標準回覆信件；當你需要直接產出可寄出的退貨回覆時使用。
---

# 目的
收到客戶退貨申請時，判斷是否在 7 天內，並回覆對應的標準信件。

# 使用時機
當輸入是一則退貨申請訊息，且內容提供了判斷是否在 7 天內所需的日期資訊時使用。

# 執行方式
1. 讀取輸入中的退貨申請內容。
2. 找出判斷是否在 7 天內所需的日期資訊。
3. 依輸入可得資訊判斷屬於「在 7 天內」或「超過 7 天」。
4. 輸出對應的標準回覆信件。
5. 只輸出信件本身，不輸出分析、計算過程或額外說明。

# 必守規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- 只根據輸入中的日期資訊判斷，不延伸推測其他條件。
- 只產生標準信件，不加入輸入未提供的事實或政策內容。

# 輸出要求
- 直接輸出可寄出的標準回覆信件。
- 信件格式要完整，不能改寫成摘要、清單或判斷報告。
- 若輸入沒有足夠日期資訊，回覆中只能明確寫出缺少的資訊為 not given。