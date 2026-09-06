---
name: excel-customer-list-cleanup
description: 處理 Excel 客戶名單時，去除重複列並標記缺少電話的列；當你要把客戶名單整理成可回貼 Excel 的乾淨表格時使用。
---

# Excel 客戶名單整理

## 你要做的事
處理使用者給你的 Excel 客戶名單，去除重複列，並標記缺少電話的列。

## 執行方式
1. 直接讀取使用者提供的名單內容。
2. 找出重複列並去除重複。
3. 找出缺少電話的列並加上明確標記。
4. 保留可直接貼回 Excel 的表格形狀。
5. 輸出整理後的結果本身。

## 規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- 去重時，只依使用者提供的內容判斷重複；如果重複判定所需資訊未給，寫 `not given`。
- 缺少電話的列要清楚標記，例如在新增欄位寫 `缺少電話` 或在該列加註 `缺少電話`。
- 不要改動未重複且電話完整的資料內容。
- 不要額外猜測欄位、補資料或延伸說明。

## 輸出格式
- 以表格輸出整理後的名單。
- 若原始資料本來就是表格，維持同樣的欄位順序與表格外觀。
- 若某項資訊缺失，輸出 `not given`。

## 完成標準
- 重複列已移除。
- 缺少電話的列已標記。
- 結果仍可直接貼回 Excel 使用。