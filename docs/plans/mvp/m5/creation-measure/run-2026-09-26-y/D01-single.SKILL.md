---
name: employee-expense-claim-flow
description: 'Use this skill when you need to process an employee expense reimbursement request according to the flowchart: check invoice completeness, handle missing documents, route large claims for manager approval, then post to finance and send the payment notice.'
---

# 員工報帳申請處理流程

依照下列流程處理每一筆員工報帳申請。不要跳步；每個判斷都要先確認再往下走。

## 1. 收到申請
- 先確認已收到完整的員工報帳申請資料。
- 建立或更新該申請的處理狀態，準備進行審核。

## 2. 檢查發票是否齊全
- 檢查申請附件中的發票是否完整、可辨識、且與報帳內容相符。
- **如果發票不齊全**：
  - 退回申請。
  - 明確要求申請人在 **3 天內補件**。
  - 暫停後續審核，直到補件完成後再重新進入流程。
- **如果發票齊全**：
  - 直接進入下一步金額判斷。

## 3. 判斷金額是否超過 5000
- 檢查本次報帳金額是否 **超過 5000**。
- **如果超過 5000**：
  - 送經理簽核。
  - 經理核准後，繼續下一步。
- **如果未超過 5000**：
  - 不需經理簽核。
  - 直接進入財務入帳步驟。

## 4. 財務直接入帳
- 將已通過審核的報帳資料交由財務直接入帳。
- 確認入帳完成後，再進行通知。

## 5. 寄出付款通知信
- 入帳完成後，寄出付款通知信給申請人。
- 通知內容應包含：申請已處理完成、付款/入帳結果，以及必要的後續說明。

## 6. 流程結束條件
- 當付款通知信已寄出，即視為此筆報帳流程完成。
- 若在任何一步發現資料不符或缺件，先依對應分支處理，不要直接進入入帳或通知步驟。
