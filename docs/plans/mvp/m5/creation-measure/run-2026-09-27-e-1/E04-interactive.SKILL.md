---
name: bulk-black-white-copy-quote
description: 計算影印店大宗黑白影印報價；當單次影印張數達 200 張以上時自動套用八折。適合用在需要快速算出原價、折扣狀態與最終總價的情境。
---

# Bulk Black-and-White Copy Quote

你會收到一個或多個黑白影印張數。你的工作是直接計算報價，並清楚輸出原價、是否套用折扣、以及最終總價。

## 規則
- 黑白影印單價為每張 1.2 元。
- 單次影印張數 **200 張以上（含）** 時，總價 **全部打八折**。
- 未達 200 張時，不套用折扣。

## 操作步驟
1. 從使用者訊息中找出要計算的張數。
2. 對每一筆張數計算原價：張數 × 1.2。
3. 若張數大於等於 200，將原價乘以 0.8，並標示「已套用八折」。
4. 若張數小於 200，維持原價，並標示「未套用折扣」。
5. 輸出每一筆的：張數、單價、原價、折扣狀態、最終總價。
6. 若輸入包含多筆資料，逐筆列出結果。

## 輸出要求
- 用清楚、可直接拿去報價的格式輸出。
- 不要省略折扣狀態。
- 不要自行更改價格規則。
- 如果輸入中出現無法判斷的張數格式，請明確說明無法使用，並請對方改成可辨識的張數。

## 注意
- 只處理黑白影印報價。
- 不要加入未提供的費用或條件，例如雙面、彩色、裝訂、運費或稅金。

## Scripts

Run each script below from this Skill's directory (the directory holding this SKILL.md) with the inputs taken from the message, and present what it prints; never work its result out yourself. `python <script> --help` lists its arguments.

- `python scripts/calc_bulk_black_white_copy_quote.py`
