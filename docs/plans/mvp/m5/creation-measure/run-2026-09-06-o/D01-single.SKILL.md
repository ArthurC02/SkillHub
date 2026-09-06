---
name: employee-expense-claim-processing
description: Use this skill when you need to process an employee expense reimbursement request shown in the flowchart, including invoice validation, missing-document follow-up, approval routing, accounting entry, and payment notification.
---

# 員工報帳申請處理流程

依照流程圖逐步處理員工報帳申請，不要跳步。

## 1. 收到申請
- 先確認已收到完整的員工報帳申請資料。
- 進入發票檢查。

## 2. 檢查發票是否齊全
- 判斷申請是否附齊所需發票。
- **如果發票齊全**：直接進入「金額是否超過 5000」的判斷。
- **如果發票不齊全**：
  - 退回申請。
  - 要求申請人在 **3 天內補件**。
  - 補件後，再重新進入「金額是否超過 5000」的判斷。

## 3. 判斷金額是否超過 5000
- 檢查報帳金額是否 **超過 5000**。
- **如果超過 5000**：
  - 送經理簽核。
  - 經理核准後，進入財務入帳。
- **如果未超過 5000**：
  - 不需經理簽核。
  - 直接進入財務入帳。

## 4. 財務直接入帳
- 由財務完成入帳作業。
- 確認入帳完成後，進入通知步驟。

## 5. 寄出付款通知信
- 寄出付款通知信給申請人。
- 流程結束。

## 執行原則
- 任何分支都要回到流程圖指定的下一步，不可自行新增審核節點。
- 若發票不齊全，必須先退回並要求 3 天內補件，不能直接進入金額判斷以外的流程。
- 若金額未超過 5000，直接由財務入帳，不需經理簽核。
