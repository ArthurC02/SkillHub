---
name: excel-customer-list-deduper
description: Clean an Excel customer list by removing duplicate rows and marking rows with missing phone numbers. Use this when the input is a customer roster that needs deduplication and a missing-phone flag in one pass.
---

# Excel 客戶名單去重與缺電話標記

你會收到一份客戶名單資料。你的任務是直接產出整理後的結果：去除重複列，並標記缺少電話的列。

## 必做規則
- 只使用輸入中提供的內容；**use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;**
- 直接交付完成的成品；**deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.**

## 處理方式
1. 讀取輸入中的表格或名單內容。
2. 依輸入明示的欄位與資料進行去重；若輸入沒有說明重複判定規則，就以輸入中可辨識為相同的列作為重複，並在結果中保留一筆。
3. 找出電話欄位缺漏的列，並依輸入要求加上明確標記。
4. 輸出整理後的完整結果，讓使用者可直接使用。

## 輸出要求
- 保留原始資料中可辨識的欄位內容。
- 清楚呈現哪些列已被移除為重複。
- 清楚標記缺少電話的列。
- 如果輸入資料本身對欄位名稱、判定規則或標記方式沒有說明，就寫 `not given`，不要自行補充。

## 產出原則
- 以最終整理結果為主，不輸出分析過程。
- 如果輸入同時提供多種表格格式，就維持輸入最容易回填的格式。
- 如果輸入不足以安全判定某一欄是否為電話，則把該欄視為 `not given`，並在輸出中如實反映。
