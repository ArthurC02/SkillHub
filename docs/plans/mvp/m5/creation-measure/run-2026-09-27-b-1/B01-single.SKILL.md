---
name: parking-fee-calculator
description: 'Use when you need to calculate a parking fee from a parking duration using a tiered pricing rule: 1 hour or less, over 1 to 3 hours, or over 3 hours capped.'
---

# 停車費試算

依照停留時間計算停車費，規則如下：

- 1 小時內：50 元
- 超過 1 小時到 3 小時：120 元
- 超過 3 小時：200 元封頂

## 計算步驟

1. 先確認停留時間的單位。
   - 若使用者提供的是小時，直接判斷。
   - 若使用者提供的是分鐘，先換算成小時再判斷。
2. 套用分級規則：
   - `停留時間 <= 1 小時` → 收 50 元
   - `1 小時 < 停留時間 <= 3 小時` → 收 120 元
   - `停留時間 > 3 小時` → 收 200 元
3. 回覆結果時，直接給出應收金額；若需要，也可簡短說明落在哪一個級距。

## 判斷注意事項

- 「1 小時內」包含剛好 1 小時。
- 「超過 1 小時到 3 小時」包含剛好 3 小時。
- 「超過 3 小時」一律以 200 元計，不再累加。
- 若使用者提供的時間格式不清楚，先請對方補充停留時間與單位，再計算。
