---
name: employee-expense-claim-processing
description: Use this skill when you need to process an employee expense reimbursement request shown in the flowchart, including checking invoice completeness, handling missing documents, routing large claims for approval, and issuing payment notice.
---

# 員工報帳申請處理流程

依照下列流程處理員工報帳申請；每一步都要先判斷條件，再執行對應動作。

## 1. 收到申請
- 接收員工報帳申請後，先進入發票檢查。

## 2. 檢查發票是否齊全
- **如果發票齊全**：直接進入「金額是否超過 5000」的判斷。
- **如果發票不齊全**：
  - 退回申請。
  - 要求申請人在 **3 天內補件**。
  - 補件後，再回到「金額是否超過 5000」的判斷。

## 3. 判斷金額是否超過 5000
- **如果金額超過 5000**：
  - 送經理簽核。
  - 經理簽核完成後，交由財務直接入帳。
- **如果金額未超過 5000**：
  - 直接交由財務入帳，不需經理簽核。

## 4. 財務入帳
- 財務完成入帳後，進入通知階段。

## 5. 寄出付款通知信
- 完成入帳後，寄出付款通知信給申請人。

## 6. 執行原則
- 不要跳過任何判斷節點。
- 若發票不齊全，必須先退回並要求 3 天內補件，再繼續後續流程。
- 金額門檻固定以 **5000** 為準。
- 超過 5000 才需要經理簽核；未超過則直接入帳。
