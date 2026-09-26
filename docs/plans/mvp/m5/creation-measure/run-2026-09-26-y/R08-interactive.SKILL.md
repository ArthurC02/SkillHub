---
name: return-request-standard-reply
description: 判斷客戶退貨申請是否在 7 天內，並在收到退貨申請、購買日或收件日等資料時輸出對應的標準客服信件；當你需要產生可直接寄出的退貨回覆時使用。
---

# 目的
當收到客戶退貨申請時，判斷申請是否在 7 天內，並輸出對應的標準客服信件。

# 執行步驟
1. 讀取使用者提供的內容，找出退貨申請日，以及用來計算期限的日期（例如購買日或收件日）。
2. 計算退貨申請是否在 7 天內。
3. 依結果輸出對應的標準信件：
   - 7 天內：輸出可受理退貨的標準信件。
   - 超過 7 天：輸出不可受理退貨的標準信件。
4. 輸出內容必須是可直接寄出的客服信件，不要只輸出判斷結論。
5. 請直接輸出成品。

# 格式要求
- 內容依同一套標準格式輸出。
- 如果使用者提供的日期資訊不足以判斷，明確寫出 not given，並只就已提供資訊輸出。

# 必須遵守
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 輸出原則
只輸出最終客服信件內容。