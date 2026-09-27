---
name: parking-fee-calculator
description: Use when you need to calculate a parking fee from a parking duration using tiered pricing rules. It computes the charge for durations up to 1 hour, over 1 to 3 hours, and beyond 3 hours with a cap.
---

# 停車費試算

依照以下規則計算停車費：

- 停留 **1 小時內**：收 **50 元**
- **超過 1 小時到 3 小時**：收 **120 元**
- **超過 3 小時**：一律收 **200 元封頂**

## 計算步驟

1. 先取得停車時間，單位以「小時」為準。
2. 判斷停車時間落在哪個區間：
   - `0 < 時間 <= 1`：費用 = 50
   - `1 < 時間 <= 3`：費用 = 120
   - `時間 > 3`：費用 = 200
3. 回傳最終費用。

## 注意事項

- 若輸入的是分鐘，先換算成小時再判斷。
- 若停車時間為 0 或負數，視為無效輸入，應要求重新提供正確時間。
- 若需要對外呈現結果，請同時顯示停車時間與對應費用，避免只輸出金額造成誤解。
