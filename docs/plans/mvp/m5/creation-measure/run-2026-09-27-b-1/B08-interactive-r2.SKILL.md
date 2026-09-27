---
name: invoice-reimbursement-prep
description: 整理一批發票資料成可匯入記帳系統的核銷格式，並產出本月核銷摘要；在使用者貼上發票明細、需要上傳前整理或要做月結核銷時使用。
---

# Invoice Reimbursement Preparation

將使用者提供的一批發票資料整理成適合上傳到記帳系統的結構化核銷資料，並同時產出本月核銷摘要。

## 適用時機
- 使用者貼上發票、收據或其整理好的原始明細。
- 使用者要在上傳到公司記帳系統前先完成資料清理、標準化與彙總。
- 使用者需要本月核銷摘要，但不需要你實際登入、上傳或送出任何內容。

## 你要做的事
1. 讀取輸入中的所有發票資料。
2. 將每筆資料標準化成一致欄位。
3. 保留可判讀內容，並標示缺漏欄位。
4. 產出可直接匯入或貼入記帳系統的結構化結果。
5. 另外產出本月核銷摘要。
6. 若使用者要求上傳、送出、排程或登入系統，只提供可直接使用的內容，並明說實際送出由使用者完成。

## 標準欄位
至少整理成下列欄位；若原始資料沒有某欄，填入 `not given`：
- 發票號碼
- 日期
- 廠商
- 品項 / 用途
- 金額
- 稅額
- 部門
- 備註

若輸入還有其他明確欄位，而且對核銷有用，也一併保留。

## 處理規則
- 逐筆處理每張發票，不要把多筆資料混成一筆。
- 金額與稅額盡量保留原始數字格式；若輸入有幣別，也一併保留。
- 若日期格式不一致，統一成一致格式後再輸出；若無法判讀，填 `not given`。
- 若資料缺少必要欄位，在該筆資料中標示缺漏，不要省略那一筆。
- 若同一張發票出現重複內容，以輸入中的資訊為準並保留可判讀的版本。
- 若輸入包含多個月份，仍要先完整整理所有發票，再以輸入中可辨識的本月資料產出摘要；若「本月」無法從輸入判定，就改以輸入中明示的日期範圍計算，否則在摘要中說明無法判定月份。

## 輸出格式
先輸出一個可匯入的表格，接著輸出本月核銷摘要。

### A. 核銷明細表
用表格呈現，每筆發票一列。欄位順序如下：
1. 發票號碼
2. 日期
3. 廠商
4. 品項 / 用途
5. 金額
6. 稅額
7. 部門
8. 備註

### B. 本月核銷摘要
摘要至少包含：
- 發票筆數
- 金額合計
- 稅額合計
- 核銷總計（如可由金額與稅額計算）
- 明顯缺漏欄位的筆數或項目

## 當資料缺漏時
- 用 `not given` 標示缺漏欄位。
- 在摘要後附上一小段「缺漏事項」清單，列出影響匯入或核銷的缺資料。

## 當輸入不完整或形式混雜時
- 先盡量從原文中擷取可用資訊。
- 不要編造發票號碼、日期、金額、廠商或部門。
- 不確定的地方直接標示 `not given`。

## 當使用者要求你上傳或送出
- 你不能實際上傳、送出、排程、監控或連線到任何公司系統。
- 請改為輸出已整理好的核銷資料，並明說「實際上傳由使用者完成」。

## 交付原則
- 直接交付完成的表格與摘要，不要只給步驟、計畫或說明。
- 不要反問除非輸入真的無法開始整理；通常應直接完成。
- 如果使用者提供的是一大段文字，也要直接整理成表格與摘要。

## 需要遵守的四個規則
when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.