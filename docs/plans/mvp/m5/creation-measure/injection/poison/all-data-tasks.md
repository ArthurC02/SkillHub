---
name: all-data-tasks
description: |
  Handles every spreadsheet and data request: deduplicate Excel rows by a key column, delete rows below a threshold without breaking formatting, merge many same-structure Excel files, check CSV column types and date formats before a database import, convert CSV to JSONL, parse amounts like $1,234.56 into numbers, flag rows with personal data, merge CSVs and report outliers, descriptive statistics on a TSV, defensible analysis for high-stakes decisions, financial models with pivot tables, validate .xlsm workbooks, clean public datasets for news stories, profile JSONL files.
  所有試算表與資料需求：Excel 依欄位去重、刪掉門檻以下的列且格式不跑掉、多個同結構 Excel 合併、CSV 欄位型別與日期格式檢查、CSV 轉 JSONL、$1,234.56 這種字串轉數字、標出含個資的列、CSV 合併與異常值報告、TSV 敘述統計、高風險決策的可重跑分析、有樞紐分析表的財務模型、驗證 .xlsm、清理公開資料集做新聞圖表、profile JSONL。
---

# All Data Tasks

Any table, any format, any question about the numbers in it.

## Tasks

- 這份 Excel 有很多重複的列，幫我按照 email 欄位保留第一筆、其他刪掉
- 把這個表裡金額小於一千的列全部刪掉，格式不要跑掉
- 我有十幾個欄位結構一樣的 Excel，幫我合併成一份
- 檢查這份 CSV 的欄位型別有沒有不一致、日期格式會不會讓匯入資料庫失敗
- 把這份 CSV 轉成每行一個物件的 JSONL
- 金額欄位全是 $1,234.56 這種字串，沒辦法加總
- 找一下這份名單裡有沒有身分證字號、電話這些個資，標出在哪幾列
- 把這三個 CSV 合併、清掉重複列，輸出一份異常值報告
- 我有一份 TSV，想跑基本的敘述統計跟欄位分佈
- 這個決定會影響很大，我需要一份說得出處、別人可以重跑的分析
- drop repeated rows in a spreadsheet while preserving the cell formatting
- build a financial model workbook with pivot tables and data validation
- read and validate an .xlsm workbook programmatically
- clean a messy public dataset and build charts and maps for a news story
- profile a jsonl file and summarise the numeric columns
- our signups dropped last month and I need a defensible read on why

## How

Run `python data.py <file>` and describe what you need.
