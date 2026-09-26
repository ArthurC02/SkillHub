---
name: customer-complaint-categorization-summary
description: 將使用者提供的客服客訴內容整理成分類統計表；適用於需要依文字內容快速彙整客訴類型、件數與代表性摘要時。
---

# 客訴分類統計表

你會收到一批客服客訴內容。你的工作是把它們整理成分類統計表。

## 必須遵守
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 工作方式
1. 先讀取使用者提供的客訴內容。
2. 依內容中可直接支持的主題進行分類。
3. 統計每一類的件數。
4. 為每一類保留一句代表性摘要或一個範例主旨。
5. 只根據輸入中的內容輸出結果，不使用外部資料。
6. 以中文輸出分類統計表。

## 分類原則
- 優先依文字中明確可見的主題分類，例如：配送延誤、商品損壞、尺寸不符、App 異常、客服回覆延遲。
- 若一則客訴無法清楚歸入任何既有類別，建立一個最保守、可直接由文字支持的類別。
- 若某欄位資訊沒有出現在輸入中，寫 `not given`，不要補寫推測內容。

## 輸出格式
以表格輸出，至少包含以下欄位：
- 類別
- 件數
- 代表性摘要或範例主旨

如有需要，也可以在表格前加上一句簡短總結，但不要加入未提供的背景說明。

## 範例處理重點
- 把每一封客訴分到最合適的一類。
- 同一類可包含多封內容相近的客訴。
- 不要捏造不存在的來源、時間範圍或處理結果。