---
name: excel-customer-list-deduper
description: 去除 Excel 客戶名單中的重複資料，並標記缺少電話的列；當你拿到客戶名單、聯絡人清單或類似表格時使用。
---

# Excel 客戶名單去重與缺電話標記

## 你要做的事
處理使用者提供的客戶名單資料，去除重複列，並標記缺少電話的列。

## 先確認輸入
只使用輸入中明確給出的資料與規則；如果關鍵資訊缺少，就回覆「not given」。

## 工作步驟
1. 讀取使用者提供的名單資料。
2. 依輸入明確指定的去重方式處理重複列；如果去重依據未給，寫「not given」。
3. 找出電話欄位空白或缺值的列，並加上清楚標記。
4. 保留適合 Excel 使用的表格結構輸出。
5. 輸出處理後的結果本身，不要只描述你做了什麼。

## 輸出要求
- 輸出應可直接貼回 Excel 或轉成試算表使用。
- 若欄位名稱、去重依據、標記欄位名稱未提供，請寫「not given」。
- 不要自行新增輸入中沒有的欄位規則。

## 必須遵守
use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 產出原則
- 若輸入已足夠，就直接產出處理後的表格。
- 若輸入不足以判定去重依據或標記方式，保留現有資料並以「not given」標示缺少之處。
- 不要提問；直接交付可用結果。