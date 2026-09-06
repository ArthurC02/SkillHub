---
name: employee-expense-reimbursement-flow
description: Use when you need to follow or automate the approval flow for an employee expense reimbursement request shown in the diagram, including invoice completeness checks, amount-based escalation, and payment notification steps.
---

# 員工報帳申請處理流程

依照流程圖處理員工報帳申請，按順序完成判斷與後續動作。

## 處理步驟

1. **收到員工報帳申請**
   - 先確認已收到完整的報帳申請資料。

2. **檢查發票是否齊全**
   - 如果**發票齊全**：直接進入下一個判斷。
   - 如果**發票不齊全**：
     - 退回申請。
     - 要求申請人在 **3 天內補件**。
     - 補件後，再重新進行「金額是否超過 5000」的判斷。

3. **判斷金額是否超過 5000**
   - 如果**超過 5000**：
     - 送經理簽核。
     - 經理核准後，交由財務直接入帳。
   - 如果**未超過 5000**：
     - 不需經理簽核。
     - 直接交由財務入帳。

4. **財務直接入帳**
   - 財務完成入帳作業。

5. **寄出付款通知信**
   - 入帳完成後，寄出付款通知信給申請人。

## 執行原則

- 發票不齊全時，先退回補件，不可直接進入入帳。
- 金額是否超過 5000 是決定是否需要經理簽核的唯一門檻。
- 不論是否需要經理簽核，最後都要由財務入帳並寄出付款通知信。
