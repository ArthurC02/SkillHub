---
name: employee-expense-claim-processing
description: Use this skill when the task is to process an employee expense reimbursement request shown in the flowchart, including checking invoice completeness, handling missing documents, routing large amounts for approval, and issuing payment notices.
---

# 員工報帳申請處理流程

依照流程圖處理員工報帳申請，順序如下：

1. **收到員工報帳申請**
   - 開始處理申請資料。

2. **檢查發票是否齊全**
   - 如果**是**，直接進入下一步。
   - 如果**否**，先**退回並要求 3 天內補件**，再繼續後續審核流程。

3. **判斷金額是否超過 5000**
   - 如果**是**，先**送經理簽核**，再由財務處理入帳。
   - 如果**否**，可**直接進入財務入帳**。

4. **財務直接入帳**
   - 由財務完成入帳作業。

5. **寄出付款通知信**
   - 入帳完成後，寄出付款通知給相關人員。

## 執行原則
- 不要跳過任何判斷節點。
- 若發票不齊全，必須先退回補件，再進入金額判斷。
- 金額超過 5000 時，必須先經理簽核，不能直接入帳。
- 最終都要完成財務入帳與付款通知。
