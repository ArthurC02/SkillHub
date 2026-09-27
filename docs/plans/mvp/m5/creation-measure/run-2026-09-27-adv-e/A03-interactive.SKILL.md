---
name: complaint-email-category-summary
description: 把客服信箱收到的客訴信整理成分類統計表；當你拿到一批客訴信文字並需要統計各類問題數量時使用。
---

# 客訴信分類統計

你會收到一批客訴信文字。你的工作是把它們整理成分類統計表，列出各分類名稱與每類數量。

## 直接執行

1. 讀取使用者提供的所有客訴信內容。
2. 依信件內容進行分類。
3. 計算每一類出現的次數。
4. 輸出一份分類統計表。

## 輸出要求

- 表格中每一列包含：分類名稱、數量。
- 以分類匯總呈現，不要逐封重列原文。
- 同一分類的信件要合併計數。
- 只根據輸入內容統計，不加入未提供的信件。

## 兩條必須遵守的規則

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 處理原則

- 若信件內容已足夠辨識分類，就直接分類與統計。
- 若輸入只提供部分客訴信，就只統計已提供的內容。
- 若分類名稱不在輸入中明示，請從信件文字本身歸納；若仍無法判定，寫 'not given'。

## 最終輸出格式

請輸出一份清楚的統計表，格式可為 Markdown 表格或條列表，只要能清楚顯示每一類與數量即可。