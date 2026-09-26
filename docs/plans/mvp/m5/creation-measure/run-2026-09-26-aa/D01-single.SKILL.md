---
name: employee-expense-claim-processing
description: Use this skill when the task is to process an employee expense reimbursement request from a flowchart or when you need to turn the chart into step-by-step handling rules. It covers invoice completeness checks, return-for-completion handling, approval routing, direct posting, and payment notification.
---

# 員工報帳申請處理流程

依照流程圖處理員工報帳申請，順序如下：

1. **收到員工報帳申請**
   - 先確認已收到完整的申請資料與附件。

2. **檢查發票是否齊全**
   - 若**發票不齊全**：
     - 退回申請。
     - 要求申請人在 **3 天內補件**。
     - 補件後，回到發票齊全的檢查流程重新判定。
   - 若**發票齊全**：
     - 直接進入下一步，檢查金額。

3. **檢查金額是否超過 5000**
   - 若**金額超過 5000**：
     - 送經理簽核。
     - 經理核准後，進入財務入帳。
   - 若**金額未超過 5000**：
     - 不需經理簽核。
     - 直接進入財務入帳。

4. **財務直接入帳**
   - 財務完成入帳作業。

5. **寄出付款通知信**
   - 入帳完成後，寄出付款通知信給申請人。

## 執行原則
- 任何不符合條件的申請，先依流程退回或補件，不可跳過前置檢查。
- 金額門檻以 **5000** 為準。
- 流程的終點是完成入帳並寄出付款通知信。
