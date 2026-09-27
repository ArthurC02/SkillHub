---
name: warehouse-picking-bonus-calculator
description: Calculate warehouse picking bonuses from a piece count using progressive tiered rates and a per-shift cap. Use when the user asks to compute or explain a picking bonus based on the 1–150, 151–300, and 301+ piece tiers with a maximum of 480 per shift.
---

# 倉儲揀貨獎金計算

依照以下規則計算揀貨獎金：

- 1 到 150 件：每件 1 元
- 超過 150 件到 300 件的部分：每件 2 元
- 超過 300 件的部分：每件 3 元
- 採「累進」分段計算，不可整批套用最高費率
- 每班獎金上限為 480 元；若計算結果超過 480 元，一律以 480 元計

## 計算步驟

1. 先確認揀貨數量 `n`。
2. 分段計算：
   - 第一段：`min(n, 150) × 1`
   - 第二段：`max(min(n, 300) - 150, 0) × 2`
   - 第三段：`max(n - 300, 0) × 3`
3. 將三段金額加總。
4. 若總額大於 480，則獎金記為 480。

## 公式

`bonus = min((min(n,150) * 1) + (max(min(n,300)-150,0) * 2) + (max(n-300,0) * 3), 480)`

## 回覆要求

- 若使用者只提供件數，直接回傳最終獎金。
- 若使用者要求說明，列出分段計算過程與封頂結果。
- 若件數不是非負整數，先請使用者更正後再計算。

## 範例

- 120 件：`120 × 1 = 120`，獎金 120 元
- 200 件：`150 × 1 + 50 × 2 = 250`，獎金 250 元
- 350 件：`150 × 1 + 150 × 2 + 50 × 3 = 600`，封頂後獎金 480 元
