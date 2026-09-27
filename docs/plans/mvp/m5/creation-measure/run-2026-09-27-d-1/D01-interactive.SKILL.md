---
name: warehouse-picking-bonus-calculator
description: Calculate warehouse picking bonuses from a single shift’s pick count using cumulative tiered rates and a per-shift cap. Use this when you need a Skill that turns pick counts into bonus amounts under the specified rules.
---

# 倉儲揀貨獎金計算 Skill

你會收到一個班次的揀貨數，輸出該班次的獎金金額。

## 計算規則

1. 先用累進分段方式計算，不是整批套用最高費率。
2. 1 到 150 件的部分：每件 1 元。
3. 超過 150 件到 300 件的部分：每件 2 元。
4. 超過 300 件的部分：每件 3 元。
5. 計算完成後，若金額超過 480 元，則一律輸出 480 元。

## 計算步驟

對每一筆輸入，按以下順序計算：

1. 先計算前 150 件：`min(揀貨數, 150) × 1`
2. 再計算 151 到 300 件的部分：`max(min(揀貨數, 300) - 150, 0) × 2`
3. 再計算 301 件以上的部分：`max(揀貨數 - 300, 0) × 3`
4. 將三段金額加總。
5. 若總金額大於 480，輸出 480；否則輸出計算結果。

## 輸出格式

- 直接輸出獎金金額，單位為元。
- 若一次收到多筆班次數字，請逐筆輸出對應結果，每筆一行。
- 不要把 150 以上整批改成高費率；一定要逐段累進。

## 範例

- 150 件 → 150 元
- 151 件 → 152 元
- 300 件 → 450 元
- 301 件 → 453 元
- 400 件 → 480 元（原始計算 753 元，超過上限）
- 0 件 → 0 元

## 注意事項

- 若輸入是多筆數字，逐筆分開計算。
- 如果輸入格式不清楚，先按可辨識的數字處理；若完全無法辨識，輸出無法計算。