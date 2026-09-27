---
name: warehouse-picking-bonus-calculator
description: Calculate warehouse picking bonuses from a picked-item count using progressive tiered rates and a per-shift cap. Use when you need to compute or verify a picker’s bonus from 1–150 items at 1元 each, 151–300 items at 2元 each, and items above 300 at 3元 each, with a maximum of 480元 per shift.
---

# 倉儲揀貨獎金計算規則

依照以下規則計算揀貨獎金。這是**累進分段**計算，不是整批套用最高費率。

## 計算區間
- 1 到 150 件：每件 1 元
- 151 到 300 件：超過 150 的部分，每件 2 元
- 301 件以上：超過 300 的部分，每件 3 元
- 每班獎金上限：480 元

## 計算步驟
1. 先確認揀貨數量 `n`。
2. 若 `n <= 0`，獎金為 0 元。
3. 計算第一段：`min(n, 150) × 1`
4. 若 `n > 150`，再計算第二段：`min(n, 300) - 150` 件 × 2
5. 若 `n > 300`，再計算第三段：`n - 300` 件 × 3
6. 將三段加總後，若總額超過 480 元，則以 480 元計。

## 公式
- `bonus = min(n,150)*1 + max(min(n,300)-150,0)*2 + max(n-300,0)*3`
- `bonus = min(bonus, 480)`

## 範例
- 100 件：`100 × 1 = 100` 元
- 150 件：`150 × 1 = 150` 元
- 200 件：`150 × 1 + 50 × 2 = 250` 元
- 300 件：`150 × 1 + 150 × 2 = 450` 元
- 320 件：`150 × 1 + 150 × 2 + 20 × 3 = 510` 元，但封頂後為 `480` 元

## 使用時注意
- 必須採用分段累進，不可把 200 件、300 件、320 件直接整批套用單一費率。
- 若輸入不是整數件數，先確認是否應四捨五入、無條件捨去或依業務規則處理；在未指定時，應先向使用者確認。
