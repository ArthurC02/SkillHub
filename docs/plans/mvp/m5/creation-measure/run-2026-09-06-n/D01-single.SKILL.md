---
name: employee-expense-claim-processing
description: Use when you need to process an employee reimbursement claim from a flowchart, especially to decide whether to return for missing invoices, escalate large amounts, and complete payment notification.
---

# 員工報帳申請處理流程

依照流程圖處理員工報帳申請，按順序判斷並執行對應動作。

## 處理步驟

1. **收到員工報帳申請**
   - 開始受理申請並檢查附件與金額資訊。

2. **判斷發票是否齊全**
   - **若是**：直接進入下一步。
   - **若否**：
     - 退回申請。
     - 要求申請人在 **3 天內補件**。
     - 補件後再重新進行「發票是否齊全」與後續判斷。

3. **判斷金額是否超過 5000**
   - **若是**：
     - 送經理簽核。
     - 簽核完成後交由財務處理。
   - **若否**：
     - 直接交由財務入帳，不需經理簽核。

4. **財務直接入帳**
   - 財務完成入帳作業。

5. **寄出付款通知信**
   - 入帳完成後，寄出付款通知信給申請人。

## 執行原則

- 發票不齊全時，先退回補件，不進入金額判斷。
- 金額是否超過 5000 只在發票齊全後判斷。
- 超過 5000 的案件必須先經理簽核，再由財務入帳。
- 未超過 5000 的案件可直接由財務入帳。
- 所有完成入帳的案件都要寄出付款通知信。
