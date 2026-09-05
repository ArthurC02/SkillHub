---
name: customer-complaint-refund-flow
description: Use this skill when the task is to follow a complaint-handling flowchart for refund-related customer complaints, including escalation, replacement handling, apology gift sending, standard reply, and complaint logging.
---

# 客訴退款處理流程

依照流程圖處理客訴時，按下列步驟執行，不要自行跳步。

## 流程步驟

1. **收到客訴訊息**
   - 先確認已收到客訴內容，並整理基本資訊。

2. **判斷是否涉及退款**
   - 如果**否**：直接進入「回覆標準說明」。
   - 如果**是**：進入下一步「轉交財務處理」。

3. **轉交財務處理**
   - 將退款相關案件交由財務單位處理。

4. **判斷是否為產品瑕疵**
   - 如果**否**：直接進入「回覆標準說明」。
   - 如果**是**：進入下一步。

5. **安排換貨**
   - 為產品瑕疵案件安排換貨處理。

6. **寄出道歉小禮**
   - 在換貨安排後，寄出道歉小禮。

7. **回覆標準說明**
   - 依標準說明回覆客戶，完成對外說明。

8. **記錄到客訴表**
   - 將案件處理結果與關鍵資訊記錄到客訴表。

## 判斷規則

- **涉及退款 = 是**：先轉交財務，再繼續後續處理。
- **涉及退款 = 否**：略過財務處理，直接回覆標準說明。
- **產品瑕疵 = 是**：安排換貨，寄出道歉小禮，再回覆標準說明。
- **產品瑕疵 = 否**：略過換貨與道歉小禮，直接回覆標準說明。

## 結束條件

- 完成標準回覆後，務必記錄到客訴表，流程才算結束。
