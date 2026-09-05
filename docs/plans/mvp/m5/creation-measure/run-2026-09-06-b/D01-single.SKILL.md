---
name: employee-expense-claim-review
description: Use this skill when you need to process an employee expense reimbursement request shown in the flowchart, including checking invoice completeness, handling missing documents, escalating large amounts, and issuing payment notice.
---

# 員工報帳申請處理流程

依照下列流程處理每一筆員工報帳申請，並在每個判斷點依結果往下執行。

## 流程步驟

1. **收到員工報帳申請**
   - 先確認已收到完整的報帳申請資料。

2. **判斷發票是否齊全**
   - 如果**發票齊全**：直接進入下一個判斷。
   - 如果**發票不齊全**：
     1. 退回申請。
     2. 要求申請人在 **3 天內補件**。
     3. 補件後再回到「金額是否超過 5000」的判斷。

3. **判斷金額是否超過 5000**
   - 如果**超過 5000**：
     1. 送經理簽核。
     2. 簽核完成後，由財務直接入帳。
   - 如果**未超過 5000**：
     1. 不需經理簽核。
     2. 直接由財務入帳。

4. **寄出付款通知信**
   - 財務入帳完成後，寄出付款通知信給申請人。

## 執行原則

- 任何缺件都先退回補件，不可直接進入入帳。
- 金額判斷以 **5000** 為門檻。
- 超過門檻才需要經理簽核。
- 最終都要完成財務入帳與付款通知信寄送。
