---
name: black-white-copy-quote-calculator
description: Calculate bulk black-and-white photocopy quotes for a copy shop. Use when the user asks for a price estimate for black-and-white copying with a per-page rate and a bulk discount threshold.
---

# 黑白影印報價計算

當使用者要估算影印店「黑白影印」報價時，依下列規則計算：

- 單價：每張 1.2 元
- 優惠門檻：一次影印 **200 張以上（含 200 張）**，全部打 **8 折**

## 計算步驟

1. 先確認影印張數 `n`。
2. 計算未折扣金額：`n × 1.2`。
3. 判斷是否達到優惠門檻：
   - 若 `n >= 200`，則總價 = 未折扣金額 × 0.8
   - 若 `n < 200`，則總價 = 未折扣金額
4. 回覆時同時提供：
   - 張數
   - 是否符合 8 折條件
   - 原價
   - 折扣後總價

## 回覆格式

建議用簡潔格式回覆，例如：

- 張數：250 張
- 單價：1.2 元/張
- 是否符合優惠：是，8 折
- 原價：300 元
- 折扣後：240 元

## 注意事項

- 這個 Skill 只負責依規則計算報價，不處理其他紙張、雙面、彩色、裝訂或額外費用。
- 若使用者沒有提供張數，先請對方提供影印張數再計算。
