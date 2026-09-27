---
name: installment-payment-calculator
description: Calculate equal installment payments with any remainder assigned to the final installment. Use when the user gives a total amount and number of periods and wants the per-period payment schedule.
---

# 分期付款計算

當使用者提供「總金額」與「期數」，並要求計算每期要繳多少時，依下列規則處理：

1. 先確認輸入值
   - 總金額必須是數字。
   - 期數必須是正整數，且大於 0。
   - 若金額或期數不完整、格式不明，先請使用者補齊。

2. 計算方式
   - 先用總金額 ÷ 期數。
   - 前面各期金額相同。
   - 若無法整除，尾差全部放在最後一期。

3. 輸出規則
   - 明確列出每一期金額。
   - 若有尾差，說明最後一期與前面各期不同。
   - 若可整除，則每期金額都相同。

4. 計算格式
   - 若是整數金額，優先以整數方式計算。
   - 若是小數金額，保留使用者輸入所需的精度，並將尾差放入最後一期。

5. 回覆範例
   - 總金額 1000，分 3 期：前 2 期各 333，最後 1 期 334。
   - 總金額 1200，分 4 期：每期 300。

6. 注意事項
   - 不要平均分攤尾差到每一期。
   - 不要忽略最後一期的調整。
   - 若使用者沒有提供總金額或期數，先詢問，不要自行假設。
