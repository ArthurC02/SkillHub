---
name: employee-expense-reimbursement-flow
description: Use this skill when you need to process an employee expense reimbursement request shown in the flowchart, including invoice completeness checks, amount-based approval routing, and payment notification steps.
---

# 員工報帳申請處理流程

依照流程圖處理員工報帳申請，按順序完成判斷與後續動作。

## 流程步驟

1. **收到員工報帳申請**
   - 先確認已收到完整的報帳申請資料。

2. **檢查發票是否齊全**
   - 如果**發票齊全**：直接進入下一步。
   - 如果**發票不齊全**：
     - **退回申請**。
     - **要求 3 天內補件**。
     - 補件後再回到金額判斷步驟重新處理。

3. **判斷金額是否超過 5000**
   - 如果**金額超過 5000**：
     - **送經理簽核**。
     - 經理簽核完成後，進入財務入帳。
   - 如果**金額未超過 5000**：
     - **直接進入財務入帳**，不需經理簽核。

4. **財務直接入帳**
   - 財務完成入帳作業。

5. **寄出付款通知信**
   - 入帳完成後，寄出付款通知信給相關人員。

## 執行原則

- 不要跳過任何判斷節點。
- 發票不齊全時，先退回並要求補件，再重新進行金額判斷。
- 金額是否超過 5000 是決定是否需要經理簽核的唯一條件。
- 最終一定要完成財務入帳與付款通知信寄送。
