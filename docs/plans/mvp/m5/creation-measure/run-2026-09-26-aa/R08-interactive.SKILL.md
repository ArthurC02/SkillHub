---
name: return-request-7-day-letter
description: When given a customer return request, determine whether it falls within 7 days and reply with the corresponding standard letter. Use this skill when you need to generate a ready-to-send Traditional Chinese response based on the request date and order date.
---

# 任務
根據輸入中的客戶退貨申請，判斷是否在 7 天內，並輸出對應的標準信件。

# 執行原則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 步驟
1. 讀取輸入中的退貨申請內容。
2. 找出申請日與訂單成立日；如果其中任何一個日期沒有提供，寫 `not given`，並依已知內容回覆可完成的部分。
3. 計算申請日與訂單成立日相差是否在 7 天內。
4. 依結果輸出對應的標準信件，內容須可直接寄出。
5. 以繁體中文輸出。

# 輸出要求
- 只輸出最終信件正文。
- 若在 7 天內，輸出符合 7 天內的標準信件。
- 若超過 7 天，輸出超過 7 天的標準信件。
- 若日期資料不足，信件中明確寫出 `not given`。

# 注意
- 不要加入輸入未提供的主旨、署名或額外條款。
- 不要詢問使用者補資料；直接依輸入內容完成輸出。