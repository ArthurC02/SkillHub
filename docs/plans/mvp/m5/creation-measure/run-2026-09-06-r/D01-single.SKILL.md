---
name: employee-expense-claim-flow
description: Use this skill when you need to process an employee expense reimbursement request shown in the flowchart, including invoice completeness checks, return-for-correction handling, approval routing, and payment notification.
---

# 員工報帳申請處理流程

依照下列流程處理收到的員工報帳申請。

## 1. 收到申請
- 先確認已收到員工報帳申請。
- 進入發票檢查。

## 2. 檢查發票是否齊全
- 判斷申請附件中的發票是否齊全。
- **如果發票不齊全**：
  - 退回申請。
  - 要求申請人在 **3 天內補件**。
  - 補件完成後，重新回到「金額是否超過 5000」的判斷步驟。
- **如果發票齊全**：
  - 直接進入「金額是否超過 5000」的判斷步驟。

## 3. 判斷金額是否超過 5000
- 檢查報帳金額是否 **超過 5000**。
- **如果超過 5000**：
  - 送經理簽核。
  - 經理簽核後，交由財務直接入帳。
- **如果未超過 5000**：
  - 不需經理簽核。
  - 直接交由財務入帳。

## 4. 財務入帳
- 財務完成直接入帳作業。
- 確認入帳完成後，進入通知步驟。

## 5. 寄出付款通知信
- 寄出付款通知信給相關人員。
- 流程結束。

## 執行原則
- 任何不符合條件的申請，先依流程退回或補件，不要跳過檢查。
- 金額判斷只決定是否需要經理簽核，不影響後續財務入帳與付款通知。
