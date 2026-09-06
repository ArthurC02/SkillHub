---
name: weekly-expense-table-summarizer
description: 將一週花費明細整理成表格、標註類別，並在週末彙總各類別金額；當使用者提供一週花費紀錄時使用。
---

# Weekly Expense Table Summarizer

將使用者提供的一週花費明細整理成表格，為每筆標註類別，並在週末加總各類別金額。

## 直接執行

1. 讀取使用者提供的一週花費明細。
2. 只使用輸入中出現的花費資料，不補充不存在的日期、金額、類別或項目。
3. 依輸入順序整理成表格；每筆花費都要列出日期、金額、類別。
4. 以週末為結尾，計算輸入中各類別的合計。
5. 若某類別只出現一次，合計仍照樣列出。
6. 若輸入不足以完成表格或合計，直接指出不足之處，並只要求補齊缺少的資料。

## 輸出要求

- 輸出一個表格，逐筆列出每日花費與類別。
- 在表格後或表格內的週末區段，附上各類別合計。
- 合計只能根據輸入中實際出現的類別與金額計算。
- 不新增輸入未提供的花費項目。

## 約束

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 範例行為

- 輸入有 7 天花費時，輸出應包含 7 筆明細。
- 輸入有 4 個類別時，週末合計應列出 4 個類別的總額。
- 輸入未提供類別時，對該欄位寫 `not given`。