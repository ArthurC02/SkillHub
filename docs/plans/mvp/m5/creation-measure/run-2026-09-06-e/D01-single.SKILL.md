---
name: employee-expense-claim-processing
description: 'Use this skill when the task is to process an employee expense reimbursement request according to the flowchart: check invoice completeness, return incomplete claims for补件 within 3 days, route large claims for manager approval, then have finance post the payment and send the remittance notice.'
---

# 員工報帳申請處理流程

依照下列流程處理員工報帳申請，不要跳步。

## 1. 收到申請
- 先確認已收到員工報帳申請。
- 進入發票與金額檢查。

## 2. 檢查發票是否齊全
- 判斷申請附件中的發票是否齊全。
- **如果發票不齊全**：
  - 退回申請。
  - 要求申請人於 **3 天內補件**。
  - 補件完成後，重新回到「金額是否超過 5000」的判斷。
- **如果發票齊全**：
  - 直接進入「金額是否超過 5000」的判斷。

## 3. 檢查金額是否超過 5000
- 判斷報帳金額是否 **超過 5000**。
- **如果超過 5000**：
  - 送經理簽核。
  - 經理核准後，交由財務直接入帳。
- **如果未超過 5000**：
  - 不需經理簽核。
  - 直接交由財務入帳。

## 4. 財務入帳
- 財務完成入帳作業。
- 確認入帳結果已建立。

## 5. 寄出付款通知信
- 入帳完成後，寄出付款通知信給申請人。
- 流程結束。

## 執行原則
- 任何不符合條件的申請，先依流程退回或補件，不要直接入帳。
- 金額門檻以 **5000** 為準。
- 最終一定要完成「財務直接入帳」與「寄出付款通知信」。
