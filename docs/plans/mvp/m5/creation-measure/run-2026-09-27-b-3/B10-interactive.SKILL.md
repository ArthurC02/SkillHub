---
name: customer-service-overage-fee
description: Computes customer service phone overage fees from a call duration. Use it when you need a minute-based charge calculation with tiered rates and a free initial period.
---

# 客服電話超時費計算

你會收到一個使用者輸入，內容是客服電話的通話分鐘數。請直接計算並輸出超時費。

## 計算規則

- 前 10 分鐘免費。
- 超過 10 分鐘到 30 分鐘的部分，每分鐘收 5 元。
- 超過 30 分鐘的部分，每分鐘收 8 元。

## 你要怎麼處理輸入

1. 讀取通話總分鐘數。
2. 依分段規則計算費用。
3. 輸出總費用，單位為元。

## 計算方式

- 若分鐘數 `m <= 10`，費用為 0。
- 若 `10 < m <= 30`，費用為 `(m - 10) * 5`。
- 若 `m > 30`，費用為 `20 * 5 + (m - 30) * 8`。

## 輸出

- 只輸出計算結果，不要加解釋。
- 結果以元表示。

## 範例

- 10 分鐘 → 0 元
- 20 分鐘 → 50 元
- 30 分鐘 → 100 元
- 35 分鐘 → 140 元
- 0 分鐘 → 0 元

## 備註

- 如果輸入中出現多筆分鐘數，就逐筆計算並逐筆輸出。
- 若輸入格式不明確，先將可辨識的數字視為分鐘數再計算。