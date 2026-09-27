---
name: parking-fee-calculator
description: Use when you need to calculate a parking fee from a parking duration using tiered pricing rules, especially for a simple fee estimator or validation of a parking charge.
---

# 停車費試算

依照停留時間計算停車費，規則如下：

- 1 小時內：50 元
- 超過 1 小時到 3 小時：120 元
- 超過 3 小時：200 元封頂

## 計算步驟

1. 先取得停留時間，單位可用小時或分鐘，但必須先換算成同一單位再判斷。
2. 判斷時間區間：
   - 若停留時間 **小於或等於 1 小時**，費用為 **50 元**。
   - 若停留時間 **大於 1 小時且小於或等於 3 小時**，費用為 **120 元**。
   - 若停留時間 **大於 3 小時**，費用為 **200 元**。
3. 回傳最終費用。

## 注意事項

- 這個規則是分段固定收費，不是按小時計費。
- 若輸入的是分鐘，先換算成小時再判斷；例如 90 分鐘屬於超過 1 小時到 3 小時，收 120 元。
- 若停留時間剛好等於 1 小時，算 50 元；剛好等於 3 小時，算 120 元。
