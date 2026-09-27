---
name: cargo-freight-rate-calculator
description: Calculate cargo freight charges from package weight, using ceiling-based kilogram rounding and tiered pricing; use this when you need a portable Skill that turns a weight into the correct fee.
---

# 貨運運費計算

這個 Skill 用來把單筆包裹重量換算成運費。

## 何時使用
- 使用者給你一個包裹重量，要你直接算運費。
- 需要依「先無條件進位到整數公斤，再套用分段費率與上限」的規則輸出金額。

## 做法
1. 讀取輸入中的單筆重量，保留原始數值。
2. 以 `python scripts/calculate_freight.py` 計算：
   - 先把重量無條件進位到整數公斤。
   - 5 公斤以下（含 5）收 150 元。
   - 超過 5 公斤、20 公斤以下（含 20）收 250 元。
   - 超過 20 公斤時，先收 250 元，再對超過 20 公斤的部分每 1 公斤加收 15 元。
   - 最終金額上限為 600 元。
3. 把腳本印出的結果原樣當作答案內容。
4. 如果輸入只有重量，直接輸出費用；如果輸入同時包含多筆重量，逐筆計算並逐筆輸出。
5. 如果輸入無法解析成數字重量，回報該輸入不可用，不要自行補值或猜測。

## 輸出要求
- 輸出應清楚寫出重量進位後的公斤數與最終運費。
- 金額以新台幣元表示。
- 不要展示中間推導，除非使用者特別要求。

## 計算規則
- 任何有分段、上限、進位的判斷，都交給腳本處理。
- 腳本會輸出最終計算結果與必要說明。

## 錯誤處理
- 如果重量不是可解析的數字，或缺少重量，標示為不可用。
- 不要自行推定單位以外的資訊。