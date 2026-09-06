---
name: employee-expense-claim-flow
description: Use this skill when you need to process an employee expense reimbursement request shown in the flowchart, including invoice completeness checks, return-for-correction handling, approval routing, and payment notification.
---

# 員工報帳申請處理流程

依照流程圖處理員工報帳申請，順序如下：

1. **收到員工報帳申請**
   - 開始受理申請資料。

2. **檢查發票是否齊全**
   - 如果**發票齊全**：直接進入下一步。
   - 如果**發票不齊全**：
     - **退回申請**。
     - **要求 3 天內補件**。
     - 補件後再回到「金額是否超過 5000」的判斷。

3. **判斷金額是否超過 5000**
   - 如果**金額超過 5000**：
     - **送經理簽核**。
     - 簽核後進入財務入帳。
   - 如果**金額未超過 5000**：
     - **直接進入財務入帳**。

4. **財務直接入帳**
   - 財務完成入帳作業。

5. **寄出付款通知信**
   - 入帳完成後，寄出付款通知信給申請人。

## 執行原則
- 發票不齊全時，先退回並要求補件，不可直接入帳。
- 金額門檻以 **5000** 為準。
- 超過 5000 的案件必須先經理簽核。
- 不超過 5000 的案件可直接由財務入帳後通知付款。
