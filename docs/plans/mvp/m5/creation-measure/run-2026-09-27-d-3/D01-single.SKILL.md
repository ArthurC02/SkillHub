---
name: warehouse-picking-bonus-calculator
description: Calculate warehouse picking bonuses from a piece count using progressive tiered rates and a per-shift cap. Use when the user asks to compute or explain a picking bonus based on 1–150, 151–300, and 301+ pieces with a maximum of 480 per shift.
---

# 倉儲揀貨獎金計算

依照以下規則計算揀貨獎金：

- 1 到 150 件：每件 1 元
- 超過 150 件到 300 件的部分：每件 2 元
- 超過 300 件的部分：每件 3 元
- 採「累進」分段計算，不可整批套用最高費率
- 每班獎金上限為 480 元，計算結果超過 480 元時，一律以 480 元計

## 計算步驟

1. 先確認揀貨數量 `n`。
2. 依分段累進計算：
   - 第一段獎金 = `min(n, 150) × 1`
   - 第二段獎金 = `max(min(n, 300) - 150, 0) × 2`
   - 第三段獎金 = `max(n - 300, 0) × 3`
3. 將三段相加得到未封頂獎金。
4. 若未封頂獎金大於 480，則最終獎金為 480；否則為未封頂獎金。

## 計算公式

`獎金 = min(480, min(n,150)*1 + max(min(n,300)-150,0)*2 + max(n-300,0)*3)`

## 回答時的要求

- 若使用者只提供件數，直接回傳最終獎金。
- 若使用者要求說明，請同時列出各段件數、各段金額、未封頂總額與封頂後金額。
- 若件數不是整數或小於 0，先指出輸入不合法，並要求重新提供正確件數。

## 範例

- 120 件：`120 × 1 = 120`，獎金 120 元
- 200 件：`150 × 1 + 50 × 2 = 250`，獎金 250 元
- 350 件：`150 × 1 + 150 × 2 + 50 × 3 = 450`，獎金 450 元
- 400 件：`150 × 1 + 150 × 2 + 100 × 3 = 600`，封頂後獎金 480 元
