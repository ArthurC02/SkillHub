---
name: employee-expense-claim-flow
description: 'Use this skill when you need to process an employee expense reimbursement request according to the flowchart: check invoice completeness, request missing documents within 3 days, route large claims for manager approval, then post to finance and send the payment notice.'
---

# 員工報帳申請處理流程

依照下列流程處理每一筆員工報帳申請。流程中的判斷與動作必須按順序執行，不可跳步。

## 1. 收到申請
- 先確認已收到員工報帳申請。
- 建立或更新該申請的處理狀態，準備進入審核。

## 2. 檢查發票是否齊全
- 檢查申請附件與單據是否完整。
- 若**發票不齊全**：
  - 退回申請。
  - 要求申請人在 **3 天內補件**。
  - 補件完成後，重新回到「金額是否超過 5000」的判斷。
- 若**發票齊全**：
  - 直接進入下一步「金額是否超過 5000」的判斷。

## 3. 判斷金額是否超過 5000
- 檢查本次報帳金額是否 **超過 5000**。
- 若**超過 5000**：
  - 送經理簽核。
  - 經理核准後，進入財務入帳。
- 若**未超過 5000**：
  - 不需經理簽核。
  - 直接進入財務入帳。

## 4. 財務直接入帳
- 將核准後的報帳資料交由財務直接入帳。
- 確認入帳完成後，進入下一步。

## 5. 寄出付款通知信
- 入帳完成後，寄出付款通知信給員工。
- 至此流程結束。

## 執行原則
- 任何缺件都先退回補件，不可直接進入後續審核。
- 金額判斷只在發票齊全或補件完成後進行。
- 超過 5000 才需要經理簽核；未超過則直接由財務入帳。
- 完成入帳後一定要寄出付款通知信。
