---
name: employee-expense-reimbursement-flow
description: Handle an employee expense reimbursement request by checking invoice completeness and amount thresholds, then routing approval, posting, and payment notification. Use when you need a step-by-step workflow for processing reimbursement applications shown in the diagram.
---

# 員工報帳申請處理流程

依照下列流程處理員工報帳申請；每一步都要先確認條件，再決定下一步。

## 流程步驟

1. **收到員工報帳申請**
   - 先確認已收到完整的報帳申請資料。

2. **檢查發票是否齊全**
   - 如果**發票齊全**：直接進入下一個判斷。
   - 如果**發票不齊全**：
     - **退回申請**，並**要求 3 天內補件**。
     - 補件後，再回到「金額是否超過 5000」的判斷。

3. **判斷金額是否超過 5000**
   - 如果**金額超過 5000**：
     1. **送經理簽核**。
     2. 簽核完成後，進入財務入帳。
   - 如果**金額未超過 5000**：
     - **直接進入財務入帳**，不需經理簽核。

4. **財務直接入帳**
   - 財務完成入帳作業。

5. **寄出付款通知信**
   - 入帳完成後，寄出付款通知信給相關人員。

## 執行原則

- 不要跳過任何判斷節點。
- 發票不齊全時，先退回並要求補件，再重新進入金額判斷。
- 金額超過 5000 才需要經理簽核；未超過則直接由財務入帳。
- 最終都要完成付款通知信寄送。
