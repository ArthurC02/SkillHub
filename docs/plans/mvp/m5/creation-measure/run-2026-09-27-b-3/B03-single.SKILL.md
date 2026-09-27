---
name: installment-payment-calculator
description: Use when you need to calculate equal installment payments from a total amount and number of periods, with any remainder added to the final installment.
---

# 分期付款計算

當使用者提供「總金額」與「期數」，並要求每期應繳金額時，依下列規則計算：

1. 先將總金額除以期數。
2. 前面各期金額相同。
3. 若無法整除，尾差全部加到最後一期。
4. 回覆時同時列出：
   - 每期固定金額
   - 最後一期金額（若有尾差）
   - 總期數
   - 總金額

## 計算方式

- 令 `總金額 = A`
- 令 `期數 = N`
- 令 `商 = A ÷ N` 的整數部分
- 令 `餘數 = A mod N`

則：
- 第 1 期到第 `N-1` 期：每期 `商`
- 第 `N` 期：`商 + 餘數`

若 `餘數 = 0`，則每一期都相同。

## 回覆格式

請用清楚、簡短的方式回覆，例如：

- 每期固定繳：X
- 最後一期繳：Y
- 共 N 期，總金額 A

## 注意事項

- 若使用者沒有提供總金額或期數，先請對方補齊資料。
- 若期數不是正整數，先請對方修正。
- 若總金額不是可計算的數字，先請對方重新提供。
