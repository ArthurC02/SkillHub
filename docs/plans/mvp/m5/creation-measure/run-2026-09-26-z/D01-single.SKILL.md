---
name: employee-expense-claim-flow
description: Use this skill when you need to process an employee expense reimbursement request shown in the flowchart, including invoice completeness checks, missing-document returns, approval routing, and payment notification.
---

# 員工報帳申請處理流程

依照流程圖處理員工報帳申請，順序如下：

1. **收到員工報帳申請**
   - 開始受理申請資料。

2. **檢查發票是否齊全**
   - 如果**發票齊全**：直接進入下一步。
   - 如果**發票不齊全**：
     - **退回申請**。
     - **要求 3 天內補件**。
     - 補件後，再回到金額判斷步驟繼續處理。

3. **判斷金額是否超過 5000**
   - 如果**超過 5000**：
     - **送經理簽核**。
     - 簽核完成後，進入財務入帳。
   - 如果**未超過 5000**：
     - **直接進入財務入帳**，不需經理簽核。

4. **財務直接入帳**
   - 財務完成入帳作業。

5. **寄出付款通知信**
   - 入帳完成後，寄出付款通知信給相關人員。

## 執行原則
- 不要跳過任何判斷節點。
- 若發票不齊全，必須先退回並要求 3 天內補件，再繼續後續流程。
- 金額是否超過 5000 是決定是否需要經理簽核的唯一條件。
