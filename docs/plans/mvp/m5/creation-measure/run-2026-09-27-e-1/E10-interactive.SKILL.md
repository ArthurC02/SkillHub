---
name: warehouse-inventory-discrepancy-sorter
description: 整理電商倉庫盤點差異：當輸入每個 SKU 的系統庫存與實際盤點數量時，輸出逐 SKU 的差異數量並標示盤盈、盤虧或無差異。
---

# 電商倉庫盤點差異整理

你會收到一段文字輸入，內容包含多筆 SKU 的系統庫存數量與實際盤點數量。你的工作是逐筆比對，輸出每個 SKU 的差異數量，並標示差異方向。

## 你要做的事

1. 讀取輸入中的每一筆 SKU 資料。
2. 對每個 SKU 計算：
   - 差異數量 = 實際盤點數量 − 系統庫存數量
3. 依差異數量判斷標示：
   - 大於 0：盤盈
   - 小於 0：盤虧
   - 等於 0：無差異
4. 逐 SKU 輸出整理結果。

## 輸出原則

- 輸出要保留 SKU。
- 輸出要包含系統庫存數量、實際盤點數量、差異數量與狀態。
- 若多筆資料輸入，請逐筆列出。
- 不要自行改寫 SKU。
- 不要自行推測未提供的數值。

## 計算規則

- 差異數量一律以「實際盤點數量 − 系統庫存數量」計算。
- 正值代表盤盈，負值代表盤虧，0 代表無差異。

## 當輸入格式不完整時

如果輸入缺少必要欄位、數量不是有效整數、或資料無法辨識，請直接指出哪個 SKU 或哪一列無法使用，並說明原因。

## 工作方式

- 先從使用者訊息中取出原始資料。
- 直接依上面的規則完成整理。
- 若輸入可用，就完成輸出，不要反問。

## 需要維持的限制

- 不要發明輸入中沒有的 SKU、數字或欄位。
- 不要把盤盈、盤虧、無差異以外的狀態自行新增。
- 不要省略任何可用資料。
- 若有無法同時滿足的限制，保留硬性限制，並用一行說明你省略了什麼。

## Scripts

Run each script below from this Skill's directory (the directory holding this SKILL.md) with the inputs taken from the message, and present what it prints; never work its result out yourself. `python <script> --help` lists its arguments.

- `python scripts/check_inventory_discrepancy.py`
