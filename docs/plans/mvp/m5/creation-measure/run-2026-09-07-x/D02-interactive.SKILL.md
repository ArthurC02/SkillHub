---
name: shopify-excel-order-flow
description: 將客戶 Excel 訂單流程圖整理成可直接對外說明的中文流程描述；當你要把單一路徑的訂單處理步驟寫成 SOP、說明文字或對外公告時使用。
---

# 目標
將輸入中的流程圖內容整理成一段可直接對外說明的中文流程描述，且只呈現確認過的單一路徑步驟。

# 你要做的事
1. 先讀取使用者提供的流程內容。
2. 依序輸出這五個步驟：
   - 收到客戶 Excel 訂單
   - 轉成 CSV 格式
   - 檢查缺漏欄位
   - 匯入 Shopify 後台
   - 回覆客戶已入單
3. 保持中文表達，並保留輸入中出現的關鍵詞 `Excel`、`CSV`、`Shopify`。
4. 只寫出這條單一路徑，不要新增分支、迴圈、例外、補充步驟或替代流程。
5. 輸出成使用者要的對外說明文字；若使用者明確要求其他格式，則在不改變已確認步驟順序的前提下，改用該格式輸出。

# 必須遵守
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# 寫作原則
- 只根據輸入中提供的內容改寫，不自行補充背景。
- 若輸入缺少資訊，就寫 `not given`。
- 不要在輸出中說明你是依照規則處理；直接交付成品。

# 輸出要求
- 直接輸出整理好的中文流程描述。
- 順序必須與確認過的步驟一致。
- 結尾要落在「回覆客戶已入單」。