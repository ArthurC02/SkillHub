---
name: employee-expense-claim-processing
description: Use this skill when the task is to process an employee expense reimbursement request shown in the flowchart, including invoice validation, amount-based approval routing, accounting entry, and payment notification.
---

# 員工報帳申請處理流程

依照流程圖處理員工報帳申請。此技能只描述可執行的流程判斷與後續動作；若缺少必要資料，先補齊再往下走。

## 處理步驟

1. **收到員工報帳申請**
   - 先確認申請內容完整：申請人、發票/憑證、金額、用途或說明。

2. **檢查發票是否齊全**
   - 若**發票齊全**：直接進入下一步。
   - 若**發票不齊全**：
     - 退回申請。
     - 要求申請人在 **3 天內補件**。
     - 補件完成後，再回到金額判斷步驟。

3. **判斷金額是否超過 5000**
   - 若**金額超過 5000**：
     - 送經理簽核。
     - 經理核准後，進入財務入帳。
   - 若**金額未超過 5000**：
     - 不需經理簽核。
     - 直接進入財務入帳。

4. **財務直接入帳**
   - 財務完成入帳作業。
   - 確認入帳結果已記錄。

5. **寄出付款通知信**
   - 入帳完成後，寄出付款通知信給申請人。
   - 通知內容應包含已受理、入帳完成與付款相關資訊。

## 例外處理

- 若補件逾期未完成，維持退回狀態，待申請人重新補齊後再處理。
- 若經理未核准，停止流程並回覆申請人依內部規定處理。
- 若財務入帳失敗，先修正入帳問題，再寄出通知信。
