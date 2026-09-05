---
name: employee-expense-approval-flow
description: Use this skill when you need to process an employee expense reimbursement request shown in the flowchart, including checking invoice completeness, handling missing documents, escalating large amounts, and completing payment notification.
---

# 員工報帳申請處理流程

依照流程圖逐步處理員工報帳申請，不要自行跳步。

## 處理步驟

1. **收到員工報帳申請**
   - 先確認已收到完整的報帳申請資料。

2. **檢查發票是否齊全**
   - 若**發票齊全**，直接進入下一步。
   - 若**發票不齊全**，先**退回並要求 3 天內補件**，再繼續後續判斷。

3. **判斷金額是否超過 5000**
   - 若**金額超過 5000**，先**送經理簽核**，簽核完成後再進入財務入帳。
   - 若**金額未超過 5000**，可**直接進入財務入帳**。

4. **財務直接入帳**
   - 由財務完成入帳作業。

5. **寄出付款通知信**
   - 入帳完成後，寄出付款通知信給相關人員。

## 執行原則

- 發票不齊全時，必須先退回補件，不可直接進入簽核或入帳。
- 金額超過 5000 時，必須先經理簽核，不可直接由財務入帳。
- 最終都要完成財務入帳與付款通知信寄送。
