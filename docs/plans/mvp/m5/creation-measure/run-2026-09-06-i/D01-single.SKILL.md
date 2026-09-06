---
name: employee-expense-claim-flow
description: Use when you need to process an employee expense reimbursement request shown in the flowchart, including invoice completeness checks, missing-document follow-up, approval routing, and payment notification.
---

# 員工報帳申請處理流程

依照流程圖處理員工報帳申請，順序如下：

1. **收到員工報帳申請**
   - 開始受理申請資料。

2. **檢查發票是否齊全**
   - 若**是**：直接進入下一步。
   - 若**否**：
     - **退回並要求 3 天內補件**。
     - 補件後再回到「金額是否超過 5000」這一步重新判斷。

3. **判斷金額是否超過 5000**
   - 若**是**：
     - **送經理簽核**。
     - 簽核後進入財務入帳。
   - 若**否**：
     - 直接進入財務入帳，不需經理簽核。

4. **財務直接入帳**
   - 財務完成入帳作業。

5. **寄出付款通知信**
   - 入帳完成後，寄出付款通知信給相關人員。

## 執行原則
- 任何缺件情況都先退回補件，不可直接進入後續審核。
- 金額門檻以 **5000** 為分界。
- 流程結束條件為：完成入帳並寄出付款通知信。
