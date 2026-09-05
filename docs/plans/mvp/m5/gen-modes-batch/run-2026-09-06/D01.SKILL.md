---
name: employee-expense-claim-processing
description: Use this skill when the task is to process an employee expense reimbursement request shown in a flowchart, including checking invoice completeness, handling missing documents, routing approvals, and issuing payment notices.
---

# 員工報帳申請處理流程

依照流程圖執行，按順序判斷與處理每一筆員工報帳申請。

## 流程步驟

1. **收到員工報帳申請**
   - 先確認已收到完整的報帳申請資料。

2. **判斷發票是否齊全**
   - 如果**發票齊全**：直接進入下一步。
   - 如果**發票不齊全**：
     - 退回申請。
     - 要求申請人在**3 天內補件**。
     - 補件後再回到「金額是否超過 5000」這一步重新判斷。

3. **判斷金額是否超過 5000**
   - 如果**金額超過 5000**：
     - 送經理簽核。
     - 經理核准後，交由財務直接入帳。
   - 如果**金額未超過 5000**：
     - 不需經理簽核。
     - 直接交由財務入帳。

4. **財務直接入帳**
   - 財務完成入帳作業。

5. **寄出付款通知信**
   - 入帳完成後，寄出付款通知信給申請人。

## 執行原則

- 發票不齊全時，不可直接進入入帳或通知付款，必須先退回補件。
- 金額門檻以 **5000** 為準。
- 超過 5000 才需要經理簽核；未超過則直接由財務處理。
- 完成入帳後才寄出付款通知信。
