---
name: weekly-expense-table-summary
description: 把一週每日花費整理成表格、標示每筆支出類別，並在週末彙總各類別合計；適合用在輸入的是一週支出明細、需要輸出表格與分類合計的情況。
---

# 一週花費表格與週末分類合計

你會收到一段描述一週每日花費的文字。請把它整理成表格，為每筆支出標示類別，並在週末彙總各類別合計。

## 處理方式

1. 讀取輸入中的一週每日花費明細。
2. 把每一天的每一筆花費整理成表格列。
3. 為每筆支出標示類別。
4. 保留每日逐筆明細，不要只輸出總和。
5. 在週末輸出各類別合計。
6. 各類別合計必須根據輸入中的逐筆花費加總得到。

## 輸出格式

- 先輸出明細表格。
- 再輸出週末各類別合計。
- 表格欄位至少包含：日期、項目、金額、類別。

## 類別標示

- 以輸入內容本身可直接判定的類別為準。
- 若輸入已明示類別，就照輸入標示。
- 若輸入只提供花費項目名稱，則依項目名稱對應到表格中的類別欄位，並保持一致。
- 輸入沒有提供的資訊，寫 `not given`。

## 輸入不足時

- 如果缺少一週每日花費明細，直接指出缺少哪些內容。
- 如果缺少某筆支出的類別且無法從輸入判定，寫 `not given`。

## 內建規則

use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.